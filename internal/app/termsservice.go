package app

import "time"

// TermsVersion is the version of the terms of use the window shows. Raise
// it when the terms change in substance: everyone is asked again.
const TermsVersion = 1

// TermsAccepted reports whether the user accepted the current terms.
func (s *Service) TermsAccepted() bool {
	return s.x.Settings.Get().Terms.AckVersion >= TermsVersion
}

// AcceptTerms records that the user accepted the current terms, and when.
func (s *Service) AcceptTerms() error {
	st := s.x.Settings.Get()
	st.Terms.AckVersion = TermsVersion
	st.Terms.AckAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveSettings(st, true)
}
