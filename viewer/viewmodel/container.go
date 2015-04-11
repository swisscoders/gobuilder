package viewmodel

import "time"

type Project struct {
	Name        string
	Status      string
	BuilderInfo string
	Timestamp   time.Time
}

type BuildInfo struct {
	Project string

	Author string
	Email  string

	Commit      string
	ShortCommit string
	Builder     string
	Slave       string
	Status      string
	StatusClass string

	Repo   string
	Branch string
	Files  []string

	Duration  time.Duration
	Timestamp time.Time
}

type BuildInfoSortByDate []*BuildInfo

func (self BuildInfoSortByDate) Len() int { return len(self) }
func (self BuildInfoSortByDate) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}
func (self BuildInfoSortByDate) Less(i, j int) bool {
	return self[i].Timestamp.After(self[j].Timestamp)
}

type HeaderRow struct {
	Name      string
	NumSlaves int
}
