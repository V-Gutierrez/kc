package auth

type Authorizer interface {
	Authorize(reason string) error
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
	if err := s.authorizer.Authorize(reason); err != nil {
		return err
	}
	s.approved = true
	return nil
}
