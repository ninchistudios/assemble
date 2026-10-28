package web

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ninchistudios/assemble/internal/store"
	"github.com/ninchistudios/assemble/internal/words"
)

// noticeView is a notification as the top of a page shows it.
type noticeView struct {
	ID   int64
	Kind string
	Text string
	Meta string
	Href string // empty for a notice with nowhere to go
}

// noticesFor loads the signed-in member's live notices. A failure is logged
// and the page shown without them, rather than failing the page.
func (s *Server) noticesFor(u *store.User, f *words.Filter) []noticeView {
	today, _ := s.today()
	ns, err := s.db.Notices(u.ID, u.CanManage(), today)
	if err != nil {
		log.Printf("notices user=%d: %v", u.ID, err)
		return nil
	}
	out := make([]noticeView, 0, len(ns))
	for _, n := range ns {
		v := noticeView{ID: n.ID, Kind: n.Kind}
		switch n.Kind {
		case store.NoticeJoined:
			// Who joined is the news, so there's no "by". No link: the
			// members page is the admin's alone.
			v.Text = f.Mask(n.ActorName) + " joined the group"
			out = append(out, v)
			continue
		case store.NoticeMOTD:
			v.Text = f.Mask(n.Body)
		case store.NoticeLocked:
			v.Text, v.Href = "Locked in: "+n.Day.Short(), dayPath(n.Day)
		default:
			v.Text, v.Href = "Cancelled: "+n.Day.Short(), dayPath(n.Day)
		}
		switch {
		case n.ActorID == u.ID:
			v.Meta = "by you"
		case n.ActorName != "":
			v.Meta = "by " + f.Mask(n.ActorName)
		}
		out = append(out, v)
	}
	return out
}

// handleDismiss hides a notice from the signed-in member. The script asks
// for JSON; a plain form goes back to the page it came from.
func (s *Server) handleDismiss(w http.ResponseWriter, r *http.Request, u *store.User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w)
		return
	}
	if err := s.db.Dismiss(id, u.ID); err != nil {
		s.fail(w, "Couldn't dismiss that.", err)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]bool{"dismissed": true})
		return
	}
	http.Redirect(w, r, s.backTo(r), http.StatusSeeOther)
}

// backTo is the same-site page a form was posted from, or home. Only a path
// on our own origin is accepted, so this can't become an open redirect.
func (s *Server) backTo(r *http.Request) string {
	ref, err := url.Parse(r.Referer())
	if err != nil || !strings.HasPrefix(r.Referer(), s.cfg.BaseURL+"/") {
		return "/"
	}
	if ref.RawQuery != "" {
		return ref.Path + "?" + ref.RawQuery
	}
	return ref.Path
}
