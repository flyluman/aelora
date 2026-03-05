package ports

import "time"

type IDGenerator interface {
	New() string
}

type Clock interface {
	Now() time.Time
}
