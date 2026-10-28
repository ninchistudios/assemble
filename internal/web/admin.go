package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ninchistudios/assemble/internal/store"
	"github.com/ninchistudios/assemble/internal/words"
)

// maxGroupName keeps the header to a line or two on a 360px phone.
const maxGroupName = 60

// zones are offered as suggestions on the settings form. Any IANA name is
// accepted; these just save typing for the likely ones. Australia and New
// Zealand are listed in full because their states differ on daylight saving.
var zones = []string{
	"UTC",
	"Australia/Adelaide", "Australia/Brisbane", "Australia/Darwin", "Australia/Hobart",
	"Australia/Melbourne", "Australia/Perth", "Australia/Sydney",
	"Pacific/Auckland",
	"Asia/Bangkok", "Asia/Hong_Kong", "Asia/Jakarta", "Asia/Kolkata", "Asia/Manila",
	"Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Tokyo",
	"Europe/Amsterdam", "Europe/Berlin", "Europe/Dublin", "Europe/Lisbon", "Europe/London",
	"Europe/Madrid", "Europe/Paris", "Europe/Rome", "Europe/Stockholm",
	"Africa/Johannesburg",
	"America/Chicago", "America/Denver", "America/Halifax", "America/Los_Angeles",
	"America/Mexico_City", "America/New_York", "America/Phoenix", "America/Sao_Paulo",
	"America/Toronto", "America/Vancouver", "Pacific/Honolulu",
}

// groupSettings is what every page header shows. A read failure is logged and
// the defaults shown, rather than failing a page over its header.
func (s *Server) groupSettings() store.Settings {
	st, err := s.db.Settings()
	if err != nil {
		log.Printf("settings: %v", err)
	}
	return st
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request, u *store.User) {
	http.Redirect(w, r, "/admin/group", http.StatusSeeOther)
}

func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request, u *store.User) {
	motd, err := s.db.MOTD()
	if err != nil {
		s.fail(w, "Couldn't load the settings.", err)
		return
	}
	p := page{
		Title: "Group", Tab: "group", User: u,
		Form: s.groupSettings(), Zones: zones, Saved: r.URL.Query().Get("saved") == "1",
		MOTD: motd, MaxMessage: store.MaxMessage,
	}
	switch r.URL.Query().Get("motd") {
	case "posted":
		p.Message = "Message of the day posted."
	case "removed":
		p.Message = "Message of the day removed."
	}
	s.render(w, http.StatusOK, "group.html", p)
}

// handleMOTDPost posts, replaces or (when empty) removes the message of the
// day (#40). A new message shows to everyone again, even those who
// dismissed the last; the same text again changes nothing.
func (s *Server) handleMOTDPost(w http.ResponseWriter, r *http.Request, u *store.User) {
	msg := words.Clean(r.FormValue("motd"))
	if problem := s.checkMessage(msg); problem != "" {
		s.render(w, http.StatusBadRequest, "group.html", page{
			Title: "Group", Tab: "group", User: u, Form: s.groupSettings(), Zones: zones,
			MOTD: msg, MaxMessage: store.MaxMessage, Error: problem,
		})
		return
	}
	changed, err := s.db.SetMOTD(msg, u.ID, s.now())
	if err != nil {
		s.fail(w, "Couldn't post the message.", err)
		return
	}
	if changed {
		log.Printf("admin: user=%d set the motd (%d chars)", u.ID, utf8.RuneCountInString(msg))
	}
	if msg == "" {
		http.Redirect(w, r, "/admin/group?motd=removed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/group?motd=posted", http.StatusSeeOther)
}

func (s *Server) handleGroupPost(w http.ResponseWriter, r *http.Request, u *store.User) {
	st := store.Settings{
		GroupName:   words.Clean(r.FormValue("group_name")),
		SlotStart:   strings.TrimSpace(r.FormValue("slot_start")),
		SlotEnd:     strings.TrimSpace(r.FormValue("slot_end")),
		Timezone:    strings.TrimSpace(r.FormValue("timezone")),
		BannedWords: cleanList(r.FormValue("banned_words")),
	}
	if msg := validateSettings(&st); msg != "" {
		motd, _ := s.db.MOTD()
		s.render(w, http.StatusBadRequest, "group.html", page{
			Title: "Group", Tab: "group", User: u, Form: st, Zones: zones, Error: msg,
			MOTD: motd, MaxMessage: store.MaxMessage,
		})
		return
	}
	if err := s.db.SaveSettings(st, time.Now()); err != nil {
		s.fail(w, "Couldn't save the settings.", err)
		return
	}
	log.Printf("admin: user=%d saved group settings", u.ID)
	// Post/redirect/get, so a refresh does not resubmit.
	http.Redirect(w, r, "/admin/group?saved=1", http.StatusSeeOther)
}

// validateSettings checks and normalises st in place, returning a message for
// the admin if something is wrong.
func validateSettings(st *store.Settings) string {
	if st.GroupName == "" {
		return "Give the group a name."
	}
	if utf8.RuneCountInString(st.GroupName) > maxGroupName {
		return "Keep the group name to 60 characters or fewer."
	}
	if utf8.RuneCountInString(st.BannedWords) > words.MaxList {
		return fmt.Sprintf("Keep the language filter to %d characters or fewer.", words.MaxList)
	}
	// The new list applies to the name being saved with it.
	if words.New(st.BannedWords).Blocked(st.GroupName) {
		return blockedMsg
	}
	start, err := parseClock(st.SlotStart)
	if err != nil {
		return "Enter the start time as HH:MM, e.g. 19:00."
	}
	end, err := parseClock(st.SlotEnd)
	if err != nil {
		return "Enter the end time as HH:MM, e.g. 22:00."
	}
	// Browsers may submit 19:00:00 from a time input; store HH:MM only, and
	// compare what will be stored, so 19:00:00 and 19:00:30 count as equal.
	st.SlotStart, st.SlotEnd = start.Format("15:04"), end.Format("15:04")
	if st.SlotStart == st.SlotEnd {
		return "The start and end times can't be the same."
	}

	// LoadLocation also accepts "" and "Local", which mean UTC and the
	// server's own zone; neither is a timezone a group lives in.
	if st.Timezone == "" || st.Timezone == "Local" {
		return "Choose a timezone, e.g. Australia/Brisbane."
	}
	loc, err := time.LoadLocation(st.Timezone)
	if err != nil {
		return "That isn't a timezone name. Use the form Region/City, e.g. Australia/Brisbane."
	}
	st.Timezone = loc.String()
	return ""
}

// cleanList tidies the admin's comma-separated filter: each entry cleaned,
// empty ones dropped, joined with ", " so it reads back neatly.
func cleanList(s string) string {
	var out []string
	for _, e := range strings.Split(s, ",") {
		if e = words.Clean(e); e != "" {
			out = append(out, e)
		}
	}
	return strings.Join(out, ", ")
}

// parseClock reads HH:MM, also accepting the HH:MM:SS some browsers submit
// from a time input.
func parseClock(v string) (time.Time, error) {
	t, err := time.Parse("15:04", v)
	if err != nil {
		t, err = time.Parse("15:04:05", v)
	}
	return t, err
}

// filter is the admin's language filter as it stands now.
func (s *Server) filter() *words.Filter { return words.New(s.groupSettings().BannedWords) }

// blockedMsg is what a member sees when their text trips the filter.
const blockedMsg = "Blocked by language filter."
