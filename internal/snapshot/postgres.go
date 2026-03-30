package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarth-shah20/stasis/internal/docker"
)

type PostgresSnapshotter struct {
	Mgr      *docker.Manager
	DBName   string
	User     string // usually "postgres"
}

func NewPostgresSnapshotter(mgr *docker.Manager, dbName string) *PostgresSnapshotter {
	return &PostgresSnapshotter{
		Mgr:    mgr,
		DBName: dbName,
		User:   "postgres", // Hardcoded for now
	}
}

func (p *PostgresSnapshotter) RequiresStop() bool { return false }

func (p *PostgresSnapshotter) Save(ctx context.Context, containerName, destDir string) error {
	// pg_dump command to get the SQL data
	cmd := []string{"pg_dump", "-U", p.User, "-d", p.DBName}

	dumpOutput, err := p.Mgr.Exec(ctx, containerName, cmd)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	// Write the output to a file in the snapshot directory
	return os.WriteFile(filepath.Join(destDir, "dump.sql"), []byte(dumpOutput), 0644)
}

func (p *PostgresSnapshotter) Load(ctx context.Context, containerName, srcDir string) error {
	dumpPath := filepath.Join(srcDir, "dump.sql")
	
	// Read the dump file
	sqlData, err := os.ReadFile(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to read snapshot dump: %w", err)
	}

	// CopyToContainer takes the file from host and puts it inside the container
	// We place it in /tmp/
	if err := p.Mgr.CopyToContainer(ctx, containerName, "/tmp/", "dump.sql", sqlData); err != nil {
		return fmt.Errorf("failed to copy dump to container: %v", err)
	}

	// Run psql to execute the copied dump file
	loadCmd := []string{"psql", "-U", p.User, "-d", p.DBName, "-f", "/tmp/dump.sql"}
	_, err = p.Mgr.Exec(ctx, containerName, loadCmd)
	if err != nil {
		return fmt.Errorf("psql restore failed: %w", err)
	}

	return nil
}