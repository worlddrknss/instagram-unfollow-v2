package extraction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/worlddrknss/instagram-unfollow-v2/pkg/storage"
)

// followingFile represents the structure of following.json
type followingFile struct {
	RelationshipsFollowing []followingEntry `json:"relationships_following"`
}

type followingEntry struct {
	Title          string           `json:"title"`
	StringListData []stringListData `json:"string_list_data"`
}

// followerEntry represents an entry in followers_*.json (array format)
type followerEntry struct {
	Title          string           `json:"title"`
	MediaListData  []interface{}    `json:"media_list_data"`
	StringListData []stringListData `json:"string_list_data"`
}

type stringListData struct {
	Href      string `json:"href"`
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
}

// ParseFollowing parses the following.json file
func ParseFollowing(jsonPath string) ([]storage.Relationship, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read following.json: %w", err)
	}

	var file followingFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("unmarshal following.json: %w", err)
	}

	var out []storage.Relationship
	for _, entry := range file.RelationshipsFollowing {
		username := entry.Title
		var href string
		var ts int64
		if len(entry.StringListData) > 0 {
			href = entry.StringListData[0].Href
			ts = entry.StringListData[0].Timestamp
		}
		out = append(out, storage.Relationship{
			Username:  username,
			Href:      href,
			Timestamp: ts,
		})
	}

	return out, nil
}

// ParseFollowers parses a followers_*.json file
func ParseFollowers(jsonPath string) ([]storage.Relationship, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read followers json: %w", err)
	}

	var entries []followerEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal followers json: %w", err)
	}

	var out []storage.Relationship
	for _, entry := range entries {
		if len(entry.StringListData) > 0 {
			sld := entry.StringListData[0]
			out = append(out, storage.Relationship{
				Username:  sld.Value,
				Href:      sld.Href,
				Timestamp: sld.Timestamp,
			})
		}
	}

	return out, nil
}

// FindFollowerFiles finds all followers_*.json files in the directory
func FindFollowerFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), "followers") && strings.HasSuffix(info.Name(), ".json") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
