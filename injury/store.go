package injury

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type ReportStore interface {
	LastReport(key string) (*Report, error)
	SaveReport(report Report) error
	WasPublished(eventID string) (bool, error)
	MarkPublished(eventID string, at time.Time) error
	Summary() (StoreSummary, error)
}

type StoreSummary struct {
	LastChecked     time.Time `json:"lastChecked"`
	LastValidReport time.Time `json:"lastValidReport"`
	Source          string    `json:"source"`
	Season          int       `json:"season"`
	Week            int       `json:"week"`
	Team            TeamCode  `json:"team"`
	Opponent        TeamCode  `json:"opponent,omitempty"`
	Players         int       `json:"players"`
	LastChange      time.Time `json:"lastChange"`
	LastPublication time.Time `json:"lastPublication"`
}

type JSONReportStore struct {
	path string
}

type storeFile struct {
	Version     int                  `json:"version"`
	Reports     map[string]Report    `json:"reports"`
	Published   map[string]time.Time `json:"published"`
	LastChecked time.Time            `json:"lastChecked"`
}

func NewJSONReportStore(path string) *JSONReportStore {
	return &JSONReportStore{path: path}
}

func (s *JSONReportStore) LastReport(key string) (*Report, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	report, ok := data.Reports[key]
	if !ok {
		return nil, nil
	}
	return &report, nil
}

func (s *JSONReportStore) SaveReport(report Report) error {
	data, err := s.load()
	if err != nil {
		return err
	}
	data.LastChecked = time.Now()
	data.Reports[report.SnapshotKey()] = report
	return s.save(data)
}

func (s *JSONReportStore) WasPublished(eventID string) (bool, error) {
	data, err := s.load()
	if err != nil {
		return false, err
	}
	_, ok := data.Published[eventID]
	return ok, nil
}

func (s *JSONReportStore) MarkPublished(eventID string, at time.Time) error {
	data, err := s.load()
	if err != nil {
		return err
	}
	data.Published[eventID] = at
	return s.save(data)
}

func (s *JSONReportStore) Summary() (StoreSummary, error) {
	data, err := s.load()
	if err != nil {
		return StoreSummary{}, err
	}
	var summary StoreSummary
	summary.LastChecked = data.LastChecked
	for _, report := range data.Reports {
		if report.RetrievedAt.After(summary.LastValidReport) {
			summary.LastValidReport = report.RetrievedAt
			summary.Source = report.Source
			summary.Season = report.Season
			summary.Week = report.Week
			summary.Team = report.Team
			summary.Opponent = report.Opponent
			summary.Players = len(report.Players)
			summary.LastChange = report.RetrievedAt
		}
	}
	for _, at := range data.Published {
		if at.After(summary.LastPublication) {
			summary.LastPublication = at
		}
	}
	return summary, nil
}

func (s *JSONReportStore) load() (storeFile, error) {
	out := storeFile{Version: 2, Reports: map[string]Report{}, Published: map[string]time.Time{}}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(b, &out); err == nil && (out.Reports != nil || out.Published != nil) {
		if out.Reports == nil {
			out.Reports = map[string]Report{}
		}
		if out.Published == nil {
			out.Published = map[string]time.Time{}
		}
		return out, nil
	}
	var legacy InjuryCache
	if err := json.Unmarshal(b, &legacy); err != nil {
		return out, err
	}
	if legacy.LastReport != nil {
		report := reportFromLegacy(*legacy.LastReport)
		out.Reports[report.SnapshotKey()] = report
		if legacy.LastHash != "" {
			out.Published["legacy:"+legacy.LastHash] = legacy.LastChecked
		}
		out.LastChecked = legacy.LastChecked
	}
	return out, nil
}

func (s *JSONReportStore) save(data storeFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil && filepath.Dir(s.path) != "." {
		return err
	}
	data.Version = 2
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0644)
}

func reportFromLegacy(old InjuryReport) Report {
	referenceTime := old.Date
	if referenceTime.IsZero() {
		referenceTime = time.Now()
	}
	report := Report{
		Season:      inferSeason(referenceTime),
		SeasonType:  SeasonTypeREG,
		Week:        inferWeek(referenceTime),
		Team:        TeamMIA,
		Source:      "legacy-cache",
		SourceURL:   "legacy-cache",
		RetrievedAt: referenceTime,
	}
	for _, p := range old.Players {
		report.Players = append(report.Players, PlayerReport{
			ID:         p.Name,
			Name:       p.Name,
			Position:   p.Position,
			Injury:     p.Injury,
			Practices:  []PracticeEntry{{Day: "latest", Status: NormalizePracticeStatus(p.Status)}},
			GameStatus: NormalizeGameStatus(p.GameStatus),
		})
	}
	return report
}
