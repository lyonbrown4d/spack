package source

// Root returns the resolved local filesystem root used for reads.
func (s *LocalFS) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// RootGuard returns a guard bound to the original resolved local filesystem root.
func (s *LocalFS) RootGuard() (*LocalRootGuard, bool, error) {
	if s == nil || s.root == "" {
		return nil, false, nil
	}
	if err := s.validateRoot(); err != nil {
		return nil, true, err
	}
	return &LocalRootGuard{root: s.root, info: s.rootInfo}, true, nil
}
