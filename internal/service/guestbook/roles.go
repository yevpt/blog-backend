package guestbook

import "github.com/vpt/blog-backend/internal/service/userrole"

func (s *guestbookService) lookupRoles(userIDs []uint) (map[uint][]string, error) {
	if s.userRepo == nil {
		return map[uint][]string{}, nil
	}
	return userrole.LookupByUserIDs(s.userRepo, userIDs)
}
