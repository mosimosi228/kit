package sqlite

import "io/fs"

type Option struct {
	Path         string
	MigrationsFS fs.FS
}
