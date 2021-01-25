package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/postgres"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/chainlink/core/utils"
	"gopkg.in/guregu/null.v4"
)

//go:generate mockery --name ORM --output ./mocks/ --case=underscore

type (
	ORM interface {
		CreateSpec(ctx context.Context, db *gorm.DB, taskDAG TaskDAG, maxTaskTimeout models.Interval) (int32, error)
		CreateRun(ctx context.Context, jobID int32, meta map[string]interface{}) (int64, error)
		ProcessNextUnfinishedRun(ctx context.Context, fn ProcessRunFunc) (bool, error)
		ListenForNewRuns() (postgres.Subscription, error)
		AwaitRun(ctx context.Context, runID int64) error
		RunFinished(runID int64) (bool, error)
		ResultsForRun(ctx context.Context, runID int64) ([]Result, error)
		DeleteRunsOlderThan(threshold time.Duration) error

		FindBridge(name models.TaskType) (models.BridgeType, error)
	}

	orm struct {
		db               *gorm.DB
		config           Config
		eventBroadcaster postgres.EventBroadcaster
	}
)

var _ ORM = (*orm)(nil)

var (
	promPipelineRunErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_run_errors",
		Help: "Number of errors for each pipeline spec",
	},
		[]string{"pipeline_spec_id"},
	)
	promPipelineRunTotalTimeToCompletion = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pipeline_run_total_time_to_completion",
		Help: "How long each pipeline run took to finish (from the moment it was created)",
	},
		[]string{"pipeline_spec_id"},
	)
	promPipelineTasksTotalFinished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_tasks_total_finished",
		Help: "The total number of pipeline tasks which have finished",
	},
		[]string{"pipeline_spec_id", "task_type", "status"},
	)
)

func NewORM(db *gorm.DB, config Config, eventBroadcaster postgres.EventBroadcaster) *orm {
	return &orm{db, config, eventBroadcaster}
}

// The tx argument must be an already started transaction.
func (o *orm) CreateSpec(ctx context.Context, tx *gorm.DB, taskDAG TaskDAG, maxTaskDuration models.Interval) (int32, error) {
	var specID int32
	spec := Spec{
		DotDagSource:    taskDAG.DOTSource,
		MaxTaskDuration: maxTaskDuration,
	}
	err := tx.Create(&spec).Error
	if err != nil {
		return specID, err
	}
	specID = spec.ID

	// Create the pipeline task specs in dependency order so
	// that we know what the successor ID for each task is
	tasks, err := taskDAG.TasksInDependencyOrder()
	if err != nil {
		return specID, err
	}

	// Create the final result task that collects the answers from the pipeline's
	// outputs.  This is a Postgres-related performance optimization.
	resultTask := ResultTask{BaseTask{dotID: ResultTaskDotID}}
	for _, task := range tasks {
		if task.DotID() == ResultTaskDotID {
			return specID, errors.Errorf("%v is a reserved keyword and cannot be used in job specs", ResultTaskDotID)
		}
		if task.OutputTask() == nil {
			task.SetOutputTask(&resultTask)
		}
	}
	tasks = append([]Task{&resultTask}, tasks...)

	taskSpecIDs := make(map[Task]int32)
	for _, task := range tasks {
		var successorID null.Int
		if task.OutputTask() != nil {
			successor := task.OutputTask()
			successorID = null.IntFrom(int64(taskSpecIDs[successor]))
		}

		taskSpec := TaskSpec{
			DotID:          task.DotID(),
			PipelineSpecID: spec.ID,
			Type:           task.Type(),
			JSON:           JSONSerializable{task, false},
			Index:          task.OutputIndex(),
			SuccessorID:    successorID,
		}
		err = tx.Create(&taskSpec).Error
		if err != nil {
			return specID, err
		}

		taskSpecIDs[task] = taskSpec.ID
	}
	return specID, errors.WithStack(err)
}

// CreateRun adds a Run record to the DB, and one TaskRun
// per TaskSpec associated with the given Spec.  Processing of the
// TaskRuns is maximally parallelized across all of the Chainlink nodes in the
// cluster.
func (o *orm) CreateRun(ctx context.Context, jobID int32, meta map[string]interface{}) (runID int64, err error) {
	ctx, cancel := utils.CombinedContext(ctx, o.config.DatabaseMaximumTxDuration())
	defer cancel()

	err = postgres.GormTransaction(ctx, o.db, func(tx *gorm.DB) (err error) {
		// Create the job run
		run := Run{}

		err = tx.Raw(`
            INSERT INTO pipeline_runs (pipeline_spec_id, meta, created_at)
            SELECT pipeline_spec_id, $1, NOW()
            FROM jobs WHERE id = $2
            RETURNING *`, JSONSerializable{Val: meta}, jobID).Scan(&run).Error
		if gorm.IsRecordNotFoundError(err) {
			return errors.Errorf("no job found with id %v (most likely it was deleted)", jobID)
		} else if err != nil {
			return errors.Wrap(err, "could not create pipeline run")
		}

		runID = run.ID

		// Create the task runs
		err = tx.Exec(`
            INSERT INTO pipeline_task_runs (
            	pipeline_run_id, pipeline_task_spec_id, type, index, created_at
            )
            SELECT ? AS pipeline_run_id, id AS pipeline_task_spec_id, type, index, NOW() AS created_at
            FROM pipeline_task_specs
            WHERE pipeline_spec_id = ?`, run.ID, run.PipelineSpecID).Error
		return errors.Wrap(err, "could not create pipeline task runs")
	})
	return runID, errors.WithStack(err)
}

// TODO: Remove generation of special "result" task
// TODO: Remove the unique index on successor_id
type ProcessRunFunc func(ctx context.Context, txdb *gorm.DB, pRun Run) []TaskRunResult

// FIXME: DOX
// ProcessNextUnfinishedRun chooses any arbitrary incomplete TaskRun from the DB
// whose parent TaskRuns have already been processed.
func (o *orm) ProcessNextUnfinishedRun(ctx context.Context, fn ProcessRunFunc) (anyRemaining bool, err error) {
	// Passed in context cancels on (chStop || JobPipelineMaxTaskDuration)
	utils.RetryWithBackoff(ctx, func() (retry bool) {
		err = o.processNextUnfinishedRun(ctx, fn)
		// "Record not found" errors mean that we're done with all unclaimed
		// job runs.
		if postgres.IsRecordNotFound(err) {
			anyRemaining = false
			retry = false
			err = nil
		} else if err != nil {
			retry = true
			err = errors.Wrap(err, "Pipeline runner could not process job run")
			logger.Error(err)

		} else {
			anyRemaining = true
			retry = false
		}
		return
	})
	return anyRemaining, errors.WithStack(err)
}

func (o *orm) processNextUnfinishedRun(ctx context.Context, fn ProcessRunFunc) error {
	// FIXME: Rename this var
	// Passed in context cancels on (chStop || JobPipelineMaxTaskDuration)
	txContext, cancel := context.WithTimeout(context.Background(), o.config.DatabaseMaximumTxDuration())
	defer cancel()
	var pRun Run

	err := postgres.GormTransaction(txContext, o.db, func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").
			Where("finished_at IS NULL").
			Preload("PipelineTaskRuns.PipelineTaskSpec.PipelineSpec").
			First(&pRun).Error
		if err != nil {
			// TODO: Don't forget to update all the wording e.g. s/task run/run/g
			return errors.Wrap(err, "error finding next task run")
		}
		logger.Infow("Pipeline run started", "runID", pRun.ID)

		trrs := fn(ctx, tx, pRun)

		if err = o.updateTaskRuns(tx, trrs); err != nil {
			return errors.Wrap(err, "could not update task runs")
		}

		if err = o.updatePipelineRun(tx, &pRun, trrs); err != nil {
			return errors.Wrap(err, "could not mark pipeline_run as finished")
		}

		err = o.eventBroadcaster.NotifyInsideGormTx(tx, postgres.ChannelRunCompleted, fmt.Sprintf("%v", pRun.ID))
		if err != nil {
			return errors.Wrap(err, "could not notify pipeline_run_completed")
		}

		elapsed := time.Since(pRun.CreatedAt)
		promPipelineRunTotalTimeToCompletion.WithLabelValues(string(pRun.PipelineSpecID)).Set(float64(elapsed))

		if pRun.HasErrors() {
			promPipelineRunErrors.WithLabelValues(string(pRun.PipelineSpecID)).Inc()
		}

		return nil
	})
	if err != nil {
		return errors.Wrap(err, "while processing run")
	}
	logger.Infow("Pipeline run completed", "runID", pRun.ID)
	return nil
}

// updateTaskRuns updates multiple task runs in one query
func (o *orm) updateTaskRuns(db *gorm.DB, trrs []TaskRunResult) error {
	sql := `
UPDATE pipeline_task_runs AS ptr SET
output = updates.output,
error = updates.error,
finished_at = updates.finished_at
FROM (VALUES
%s
) AS updates(id, output, error, finished_at)
WHERE ptr.id = updates.id
`
	// NOTE: gormv1 does not support bulk updates so we have to
	// manually construct it ourselves
	valueStrings := []string{}
	valueArgs := []interface{}{}
	for _, trr := range trrs {
		valueStrings = append(valueStrings, "(?::bigint, ?::jsonb, ?::text, ?::timestamptz)")
		valueArgs = append(valueArgs, trr.ID, trr.Result.OutputDB(), trr.Result.ErrorDB(), trr.FinishedAt)
	}

	/* #nosec G201 */
	stmt := fmt.Sprintf(sql, strings.Join(valueStrings, ","))
	return db.Exec(stmt, valueArgs...).Error
}

func (o *orm) updatePipelineRun(db *gorm.DB, run *Run, trrs []TaskRunResult) error {
	var result Result
	for _, trr := range trrs {
		if trr.IsFinal {
			// FIXME: This assumes there is only one final result and will
			// have to change when the magical "__result__" type is removed
			result = trr.Result
		}
	}

	return db.Raw(`
		UPDATE pipeline_runs SET finished_at = ?, outputs = ?, errors = ?
		WHERE id = ?
		RETURNING *
		`, time.Now(), result.OutputDB(), result.ErrorDB(), run.ID).
		Scan(run).Error
}

func (o *orm) ListenForNewRuns() (postgres.Subscription, error) {
	return o.eventBroadcaster.Subscribe(postgres.ChannelRunStarted, "")
}

// AwaitRun waits until a run has completed (either successfully or with errors)
// and then returns.  It uses two distinct methods to determine when a job run
// has completed:
//    1) periodic polling
//    2) Postgres notifications
func (o *orm) AwaitRun(ctx context.Context, runID int64) error {
	// This goroutine polls the DB at a set interval
	chPoll := make(chan error)
	chDone := make(chan struct{})
	defer close(chDone)
	go func() {
		var err error
		for {
			select {
			case <-chDone:
				return
			case <-ctx.Done():
				return
			default:
			}

			var done bool
			done, err = o.RunFinished(runID)
			if err != nil || done {
				break
			}
			time.Sleep(1 * time.Second)
		}

		select {
		case chPoll <- err:
		case <-chDone:
		case <-ctx.Done():
		}
	}()

	// This listener subscribes to the Postgres event informing us of a completed pipeline run
	sub, err := o.eventBroadcaster.Subscribe(postgres.ChannelRunCompleted, fmt.Sprintf("%d", runID))
	if err != nil {
		return err
	}
	defer sub.Close()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-chPoll:
		return err
	case <-sub.Events():
		return nil
	}
}

func (o *orm) ResultsForRun(ctx context.Context, runID int64) ([]Result, error) {
	// TODO(sam): I think this can be optimised by condensing it down into one query
	// See: https://www.pivotaltracker.com/story/show/175288635
	done, err := o.RunFinished(runID)
	if err != nil {
		return nil, err
	} else if !done {
		return nil, errors.New("can't fetch run results, run is still in progress")
	}

	ctx, cancel := utils.CombinedContext(ctx, o.config.DatabaseMaximumTxDuration())
	defer cancel()

	var results []Result
	err = postgres.GormTransaction(ctx, o.db, func(tx *gorm.DB) error {
		var resultTaskRun TaskRun
		err = tx.
			Preload("PipelineTaskSpec").
			Joins("INNER JOIN pipeline_task_specs ON pipeline_task_runs.pipeline_task_spec_id = pipeline_task_specs.id").
			Where("pipeline_run_id = ?", runID).
			Where("finished_at IS NOT NULL").
			Where("pipeline_task_specs.successor_id IS NULL").
			Where("pipeline_task_specs.dot_id = ?", ResultTaskDotID).
			First(&resultTaskRun).
			Error
		if err != nil {
			return errors.Wrapf(err, "Pipeline runner could not fetch pipeline results (runID: %v)", runID)
		}

		var values []interface{}
		var errs FinalErrors
		if resultTaskRun.Output != nil && resultTaskRun.Output.Val != nil {
			vals, is := resultTaskRun.Output.Val.([]interface{})
			if !is {
				return errors.Errorf("Pipeline runner invariant violation: result task run's output must be []interface{}, got %T", resultTaskRun.Output.Val)
			}
			values = vals
		}
		if !resultTaskRun.Error.IsZero() {
			err = json.Unmarshal([]byte(resultTaskRun.Error.ValueOrZero()), &errs)
			if err != nil {
				return errors.Errorf("Pipeline runner invariant violation: result task run's errors must be []error, got %v", resultTaskRun.Error.ValueOrZero())
			}
		}
		if len(values) != len(errs) {
			return errors.Errorf("Pipeline runner invariant violation: result task run must have equal numbers of outputs and errors (got %v and %v)", len(values), len(errs))
		}
		results = make([]Result, len(values))
		for i := range values {
			results[i].Value = values[i]
			if !errs[i].IsZero() {
				results[i].Error = errors.New(errs[i].ValueOrZero())
			}
		}
		return nil
	})
	return results, err
}

func (o *orm) RunFinished(runID int64) (bool, error) {
	var done struct{ Done bool }
	// FIXME: Since we denormalised this can be made more efficient
	err := o.db.Raw(`
        SELECT finished_at IS NOT NULL AS done
        FROM pipeline_task_runs
        INNER JOIN pipeline_task_specs ON pipeline_task_runs.pipeline_task_spec_id = pipeline_task_specs.id
        WHERE pipeline_task_runs.pipeline_run_id = ? AND pipeline_task_specs.successor_id IS NULL
		LIMIT 1
    `, runID).Scan(&done).Error
	return done.Done, errors.Wrapf(err, "could not determine if run is finished (run ID: %v)", runID)
}

func (o *orm) DeleteRunsOlderThan(threshold time.Duration) error {
	err := o.db.Exec(`DELETE FROM pipeline_runs WHERE finished_at < ?`, time.Now().Add(-threshold)).Error
	if err != nil {
		return err
	}
	return nil
}

func (o *orm) FindBridge(name models.TaskType) (models.BridgeType, error) {
	return FindBridge(o.db, name)
}

// FindBridge find a bridge using the given database
func FindBridge(db *gorm.DB, name models.TaskType) (models.BridgeType, error) {
	var bt models.BridgeType
	return bt, errors.Wrapf(db.First(&bt, "name = ?", name.String()).Error, "could not find bridge with name '%s'", name)
}
