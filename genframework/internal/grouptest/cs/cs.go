// Package cs is an in-memory stand-in for the go-teams group grant and read
// helper, used to exercise the generated group assignment resource. It keeps
// Teams' semantics: ranks are dense per policy type (inserting at an occupied
// rank shifts the others down, an out-of-range rank is placed last), names
// compare case-insensitively, and writes surface after Lag reads.
package cs

import (
	"context"
	"strings"
	"sync"
)

type GrantCsTeamsMeetingPolicyParams struct {
	Group      string
	PolicyName string
	Rank       *int64
}

type entry struct{ group, name string }

// Read is one scripted read result.
type Read struct {
	Name string
	Rank int64
}

type Service struct {
	mu      sync.Mutex
	order   []entry // index+1 is the rank
	visible []entry // what reads return while a write is still propagating
	lagLeft int
	Lag     int // reads that still see the previous state after a write
	// StaleUpper returns names in upper case while a write is still propagating
	// (a stale read that differs from the pre-grant read only in case).
	StaleUpper bool
	// AfterGrant is returned by the reads right after the next grant, one
	// entry per read, before the normal view (intermediate replica states).
	AfterGrant []Read
	script     []Read
	Reads      int
	Grants     int
}

func (s *Service) GrantCsTeamsMeetingPolicy(_ context.Context, p GrantCsTeamsMeetingPolicyParams) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Grants++
	s.script, s.AfterGrant = s.AfterGrant, nil
	s.visible, s.lagLeft = append([]entry(nil), s.order...), s.Lag
	s.place(p.Group, p.PolicyName, p.Rank)
	return nil, nil
}

// Set changes the assignment outside Terraform, visible immediately.
func (s *Service) Set(group, name string, rank int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.place(group, name, &rank)
	s.lagLeft = 0
}

func (s *Service) place(group, name string, rank *int64) {
	for i, e := range s.order {
		if strings.EqualFold(e.group, group) {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	if name == "" {
		return
	}
	at := len(s.order)
	if rank != nil && *rank >= 1 && int(*rank-1) < at {
		at = int(*rank - 1)
	}
	s.order = append(s.order[:at], append([]entry{{group, name}}, s.order[at:]...)...)
}

func (s *Service) GroupPolicyAssignment(_ context.Context, group, policyType string) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Reads++
	if len(s.script) > 0 {
		r := s.script[0]
		s.script = s.script[1:]
		return r.Name, r.Rank, nil
	}
	view := s.order
	stale := s.lagLeft > 0
	if stale {
		s.lagLeft--
		view = s.visible
	}
	if policyType != "TeamsMeetingPolicy" {
		return "", 0, nil
	}
	for i, e := range view {
		if strings.EqualFold(e.group, group) {
			if stale && s.StaleUpper {
				return strings.ToUpper(e.name), int64(i + 1), nil
			}
			return e.name, int64(i + 1), nil
		}
	}
	return "", 0, nil
}
