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
