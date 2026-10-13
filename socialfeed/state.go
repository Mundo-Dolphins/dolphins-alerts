package socialfeed

import (
	"bufio"
	"os"
	"sort"
	"strings"
)

type SeenStore struct {
	path string
	ids  map[string]struct{}
}

func NewSeenStore(path string) *SeenStore {
	return &SeenStore{
		path: path,
		ids:  make(map[string]struct{}),
	}
}

func (s *SeenStore) Load() error {
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id != "" {
			s.ids[id] = struct{}{}
		}
	}
	return scanner.Err()
}

func (s *SeenStore) Save() error {
	ids := make([]string, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return os.WriteFile(s.path, []byte(strings.Join(ids, "\n")), 0600)
}

func (s *SeenStore) IsSeen(id string) bool {
	_, ok := s.ids[id]
	return ok
}

func (s *SeenStore) Mark(id string) {
	id = strings.TrimSpace(id)
	if id != "" {
		s.ids[id] = struct{}{}
	}
}

func NewPosts(posts []SocialPost, seen interface{ IsSeen(string) bool }) []SocialPost {
	var fresh []SocialPost
	for _, post := range posts {
		if !seen.IsSeen(post.ID) {
			fresh = append(fresh, post)
		}
	}
	for i, j := 0, len(fresh)-1; i < j; i, j = i+1, j-1 {
		fresh[i], fresh[j] = fresh[j], fresh[i]
	}
	return fresh
}
