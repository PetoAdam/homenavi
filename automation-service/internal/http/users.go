package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
)

type userServiceUser struct {
	ID       string `json:"id"`
	UserName string `json:"user_name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type userServiceListResponse struct {
	Users      []userServiceUser `json:"users"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

func (s *Server) fetchUser(ctx context.Context, token string, userID string) (*userServiceUser, error) {
	if s.userServiceURL == "" {
		return nil, errors.New("user service url not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user id required")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing auth token")
	}

	url := s.userServiceURL + "/users/" + userID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("user-service %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	var u userServiceUser
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, errors.New("invalid user-service response")
	}
	u.Email = strings.TrimSpace(u.Email)
	u.UserName = strings.TrimSpace(u.UserName)
	if u.Email == "" {
		return nil, errors.New("user has no email")
	}
	if u.UserName == "" {
		u.UserName = "Homenavi user"
	}
	return &u, nil
}

func (s *Server) listAllUsers(ctx context.Context, token string) ([]userServiceUser, error) {
	if s.userServiceURL == "" {
		return nil, errors.New("user service url not configured")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("missing auth token")
	}

	all := make([]userServiceUser, 0, 256)
	page := 1
	pageSize := 200
	for {
		url := fmt.Sprintf("%s/users?page=%d&page_size=%d", s.userServiceURL, page, pageSize)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("user-service %s: %s", resp.Status, strings.TrimSpace(string(b)))
		}
		var lr userServiceListResponse
		if err := json.Unmarshal(b, &lr); err != nil {
			return nil, errors.New("invalid user-service list response")
		}
		all = append(all, lr.Users...)
		if lr.TotalPages <= 0 || page >= lr.TotalPages {
			break
		}
		page++
		if page > 1000 {
			break
		}
	}
	return all, nil
}

func (s *Server) enrichNotifyEmailRecipients(ctx context.Context, token string, defBytes []byte) ([]byte, error) {
	var d engine.Definition
	if err := json.Unmarshal(defBytes, &d); err != nil {
		return nil, errors.New("definition must be valid json")
	}

	for i := range d.Nodes {
		n := d.Nodes[i]
		if strings.ToLower(strings.TrimSpace(n.Kind)) != "action.notify_email" {
			continue
		}
		var a engine.ActionNotifyEmail
		if err := json.Unmarshal(n.Data, &a); err != nil {
			return nil, errors.New("action.notify_email data must be valid json object")
		}

		seen := map[string]struct{}{}
		recips := make([]engine.NotifyEmailRecipient, 0, len(a.UserIDs))
		roleSet := map[string]struct{}{}
		for _, tr := range a.TargetRoles {
			r := strings.ToLower(strings.TrimSpace(tr))
			if r == "" {
				continue
			}
			roleSet[r] = struct{}{}
		}
		if len(roleSet) > 0 {
			users, err := s.listAllUsers(ctx, token)
			if err != nil {
				return nil, err
			}
			for _, u := range users {
				uid := strings.TrimSpace(u.ID)
				if uid == "" {
					continue
				}
				role := strings.ToLower(strings.TrimSpace(u.Role))
				if _, ok := roleSet[role]; !ok {
					continue
				}
				if _, ok := seen[uid]; ok {
					continue
				}
				seen[uid] = struct{}{}
				email := strings.TrimSpace(u.Email)
				name := strings.TrimSpace(u.UserName)
				if email == "" {
					continue
				}
				if name == "" {
					name = "Homenavi user"
				}
				recips = append(recips, engine.NotifyEmailRecipient{UserID: uid, Email: email, UserName: name})
			}
		}

		for _, rawID := range a.UserIDs {
			uid := strings.TrimSpace(rawID)
			if uid == "" {
				continue
			}
			if _, ok := seen[uid]; ok {
				continue
			}
			seen[uid] = struct{}{}
			u, err := s.fetchUser(ctx, token, uid)
			if err != nil {
				return nil, err
			}
			recips = append(recips, engine.NotifyEmailRecipient{UserID: uid, Email: u.Email, UserName: u.UserName})
		}
		a.Recipients = recips
		b, _ := json.Marshal(a)
		d.Nodes[i].Data = b
	}

	out, _ := json.Marshal(d)
	return out, nil
}
