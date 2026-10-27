package store

import "testing"

func TestSettingsDefaultsThenSave(t *testing.T) {
	s := newStore(t)
	st, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if st.GroupName != DefaultSettings.GroupName || st.SlotStart != "19:00" || st.Timezone != "UTC" || !st.UpdatedAt.IsZero() {
		t.Errorf("default settings = %+v", st)
	}

	want := Settings{GroupName: "The Band", SlotStart: "20:00", SlotEnd: "23:30", Timezone: "Australia/Brisbane", BannedWords: "darn, heck"}
	if err := s.SaveSettings(want, t0); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupName != want.GroupName || got.SlotStart != want.SlotStart || got.SlotEnd != want.SlotEnd ||
		got.Timezone != want.Timezone || got.BannedWords != want.BannedWords || !got.UpdatedAt.Equal(t0) {
		t.Errorf("saved settings = %+v", got)
	}
}
