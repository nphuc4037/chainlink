package migration1611388693

import "github.com/jinzhu/gorm"

// Migrate makes the explicit the pre-existing
// implicit assumption that lowercase external initiator names are unique
func Migrate(tx *gorm.DB) error {
	return tx.Exec(`
		CREATE INDEX idx_pipeline_runs_unfinished_runs ON pipeline_runs (finished_at) WHERE finished_at IS NULL;
		DROP INDEX idx_pipeline_task_runs_optimise_find_predecessor_unfinished_runs;
		DROP INDEX idx_pipeline_task_runs_unfinished;
	`).Error
}
