package auth

type Authorizer interface {
	Authorize(reason string) error
}

type bootSessionCapable interface {
	Authorizer
	bootSessionEnabled() bool
}

type Session struct {
	authorizer Authorizer
	approved   bool
}

func NewSession(authorizer Authorizer) *Session {
	return &Session{authorizer: authorizer}
}

func (s *Session) Authorize(reason string) error {
	if s == nil || s.authorizer == nil {
		return nil
	}
	if s.approved {
		return nil
	}
	if supportsBootSession(s.authorizer) && isBootSessionValid() {
		s.approved = true
		return nil
	}
	if err := s.authorizer.Authorize(reason); err != nil {
		return err
	}
	if supportsBootSession(s.authorizer) {
		_ = writeBootSessionToken()
	}
	s.approved = true
	return nil
}

func supportsBootSession(authorizer Authorizer) bool {
	capable, ok := authorizer.(bootSessionCapable)
	return ok && capable.bootSessionEnabled()
}
