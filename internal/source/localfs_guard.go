package source

// Root returns the resolved local filesystem root used for reads.
func (s *LocalFS) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// RootGuard returns a fresh guard bound to the resolved local filesystem root.
func (s *LocalFS) RootGuard() (*LocalRootGuard, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	return NewLocalRootGuard(s.root)
}
