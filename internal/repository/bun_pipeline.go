package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gumeniukcom/contactshq/internal/domain"
	"github.com/uptrace/bun"
)

type BunPipelineRepository struct {
	db *bun.DB
}

func NewBunPipelineRepository(db *bun.DB) *BunPipelineRepository {
	return &BunPipelineRepository{db: db}
}

func (r *BunPipelineRepository) Create(ctx context.Context, pipeline *domain.Pipeline) error {
	_, err := r.db.NewInsert().Model(pipeline).Exec(ctx)
	return err
}

func (r *BunPipelineRepository) GetByID(ctx context.Context, id string) (*domain.Pipeline, error) {
	pipeline := new(domain.Pipeline)
	err := r.db.NewSelect().Model(pipeline).
		Relation("Steps", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.OrderExpr("step_order ASC")
		}).
		Where("p.id = ?", id).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return pipeline, err
}

func (r *BunPipelineRepository) ListByUser(ctx context.Context, userID string) ([]*domain.Pipeline, error) {
	var pipelines []*domain.Pipeline
	err := r.db.NewSelect().Model(&pipelines).
		Relation("Steps", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.OrderExpr("step_order ASC")
		}).
		Where("p.user_id = ?", userID).
		OrderExpr("p.created_at DESC").
		Scan(ctx)
	return pipelines, err
}

func (r *BunPipelineRepository) Update(ctx context.Context, pipeline *domain.Pipeline) error {
	_, err := r.db.NewUpdate().Model(pipeline).WherePK().Exec(ctx)
	return err
}

func (r *BunPipelineRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.NewDelete().Model((*domain.Pipeline)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *BunPipelineRepository) CreateStep(ctx context.Context, step *domain.PipelineStep) error {
	_, err := r.db.NewInsert().Model(step).Exec(ctx)
	return err
}

func (r *BunPipelineRepository) GetSteps(ctx context.Context, pipelineID string) ([]*domain.PipelineStep, error) {
	var steps []*domain.PipelineStep
	err := r.db.NewSelect().Model(&steps).
		Where("pipeline_id = ?", pipelineID).
		OrderExpr("step_order ASC").
		Scan(ctx)
	return steps, err
}

func (r *BunPipelineRepository) DeleteSteps(ctx context.Context, pipelineID string) error {
	_, err := r.db.NewDelete().Model((*domain.PipelineStep)(nil)).Where("pipeline_id = ?", pipelineID).Exec(ctx)
	return err
}

func (r *BunPipelineRepository) ListAllEnabled(ctx context.Context) ([]*domain.Pipeline, error) {
	var pipelines []*domain.Pipeline
	err := r.db.NewSelect().Model(&pipelines).
		Where("p.enabled = ?", true).
		Where("p.schedule != ''").
		OrderExpr("p.created_at ASC").
		Scan(ctx)
	return pipelines, err
}
