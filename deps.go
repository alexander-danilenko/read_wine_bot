package main

// Temporary: these blank imports keep the chosen libraries pinned in go.mod
// before any code uses them. Delete each import as soon as real code imports
// that library, and delete this file once it is empty.
import (
	_ "github.com/glebarez/sqlite"
	_ "gorm.io/driver/postgres"
	_ "gorm.io/gorm"
)
