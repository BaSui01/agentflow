package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/agent/observability/evaluation"
	"github.com/BaSui01/agentflow/agent/persistence"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
)

// Collection names for experiment data.
const (
	experimentsCollection = "experiments"
	assignmentsCollection = "experiment_assignments"
	expResultsCollection  = "experiment_results"
)

// experimentDocument wraps evaluation.Experiment for MongoDB storage.
type experimentDocument struct {
	ID          string                      `bson:"_id"         json:"id"`
	Name        string                      `bson:"name"        json:"name"`
	Description string                      `bson:"description" json:"description"`
	Variants    []evaluation.Variant        `bson:"variants"    json:"variants"`
	Metrics     []string                    `bson:"metrics"     json:"metrics"`
	StartTime   time.Time                   `bson:"start_time"  json:"start_time"`
	EndTime     *time.Time                  `bson:"end_time"    json:"end_time,omitempty"`
	Status      evaluation.ExperimentStatus `bson:"status"      json:"status"`
	UpdatedAt   time.Time                   `bson:"updated_at"  json:"updated_at"`
}

// assignmentDocument stores a user-to-variant assignment.
type assignmentDocument struct {
	ExperimentID string `bson:"experiment_id" json:"experiment_id"`
	UserID       string `bson:"user_id"       json:"user_id"`
	VariantID    string `bson:"variant_id"    json:"variant_id"`
}

// resultDocument stores an evaluation result for a variant.
type resultDocument struct {
	ExperimentID string                 `bson:"experiment_id" json:"experiment_id"`
	VariantID    string                 `bson:"variant_id"    json:"variant_id"`
	Result       *evaluation.EvalResult `bson:"result"     json:"result"`
	RecordedAt   time.Time              `bson:"recorded_at"   json:"recorded_at"`
}

// MongoExperimentStore implements evaluation.ExperimentStore backed by MongoDB.
type MongoExperimentStore struct {
	experiments *mongo.Collection
	assignments *mongo.Collection
	results     *mongo.Collection
}

// NewExperimentStore creates a MongoExperimentStore and ensures indexes.
func NewExperimentStore(ctx context.Context, client *mongoclient.Client) (*MongoExperimentStore, error) {
	s := &MongoExperimentStore{
		experiments: client.Collection(experimentsCollection),
		assignments: client.Collection(assignmentsCollection),
		results:     client.Collection(expResultsCollection),
	}

	// Experiment indexes.
	expIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "status", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, experimentsCollection, expIndexes); err != nil {
		return nil, err
	}

	// Assignment indexes: unique per experiment+user.
	assignIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "experiment_id", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	if err := client.EnsureIndexes(ctx, assignmentsCollection, assignIndexes); err != nil {
		return nil, err
	}

	// Result indexes.
	resIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "experiment_id", Value: 1}, {Key: "variant_id", Value: 1}}},
	}
	if err := client.EnsureIndexes(ctx, expResultsCollection, resIndexes); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *MongoExperimentStore) SaveExperiment(ctx context.Context, exp *evaluation.Experiment) error {
	if exp == nil || exp.ID == "" {
		return persistence.ErrInvalidInput
	}

	doc := &experimentDocument{
		ID:          exp.ID,
		Name:        exp.Name,
		Description: exp.Description,
		Variants:    exp.Variants,
		Metrics:     exp.Metrics,
		StartTime:   exp.StartTime,
		EndTime:     exp.EndTime,
		Status:      exp.Status,
		UpdatedAt:   time.Now(),
	}

	opts := options.Replace().SetUpsert(true)
	_, err := s.experiments.ReplaceOne(ctx, bson.D{{Key: "_id", Value: exp.ID}}, doc, opts)
	return err
}

func (s *MongoExperimentStore) LoadExperiment(ctx context.Context, id string) (*evaluation.Experiment, error) {
	var doc experimentDocument
	err := s.experiments.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, evaluation.ErrExperimentNotFound
		}
		return nil, err
	}
	return docToExperiment(&doc), nil
}

func (s *MongoExperimentStore) ListExperiments(ctx context.Context) ([]*evaluation.Experiment, error) {
	opts := options.Find().SetSort(bson.D{{Key: "start_time", Value: 1}}).SetLimit(500)
	cursor, err := s.experiments.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []experimentDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	result := make([]*evaluation.Experiment, 0, len(docs))
	for i := range docs {
		result = append(result, docToExperiment(&docs[i]))
	}
	return result, nil
}

func (s *MongoExperimentStore) DeleteExperiment(ctx context.Context, id string) error {
	_, err := s.experiments.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	// Clean up related assignments and results.
	_, _ = s.assignments.DeleteMany(ctx, bson.D{{Key: "experiment_id", Value: id}})
	_, _ = s.results.DeleteMany(ctx, bson.D{{Key: "experiment_id", Value: id}})
	return nil
}

func (s *MongoExperimentStore) RecordAssignment(ctx context.Context, experimentID, userID, variantID string) error {
	doc := &assignmentDocument{
		ExperimentID: experimentID,
		UserID:       userID,
		VariantID:    variantID,
	}
	opts := options.Replace().SetUpsert(true)
	_, err := s.assignments.ReplaceOne(ctx,
		bson.D{{Key: "experiment_id", Value: experimentID}, {Key: "user_id", Value: userID}},
		doc, opts,
	)
	return err
}

func (s *MongoExperimentStore) GetAssignment(ctx context.Context, experimentID, userID string) (string, error) {
	var doc assignmentDocument
	err := s.assignments.FindOne(ctx,
		bson.D{{Key: "experiment_id", Value: experimentID}, {Key: "user_id", Value: userID}},
	).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", nil
		}
		return "", err
	}
	return doc.VariantID, nil
}

func (s *MongoExperimentStore) RecordResult(ctx context.Context, experimentID, variantID string, result *evaluation.EvalResult) error {
	doc := &resultDocument{
		ExperimentID: experimentID,
		VariantID:    variantID,
		Result:       result,
		RecordedAt:   time.Now(),
	}
	_, err := s.results.InsertOne(ctx, doc)
	return err
}

func (s *MongoExperimentStore) GetResults(ctx context.Context, experimentID string) (map[string][]*evaluation.EvalResult, error) {
	opts := options.Find().SetLimit(1000)
	cursor, err := s.results.Find(ctx, bson.D{{Key: "experiment_id", Value: experimentID}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []resultDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	results := make(map[string][]*evaluation.EvalResult)
	for _, doc := range docs {
		results[doc.VariantID] = append(results[doc.VariantID], doc.Result)
	}
	return results, nil
}

// docToExperiment converts an experimentDocument to an evaluation.Experiment.
func docToExperiment(doc *experimentDocument) *evaluation.Experiment {
	return &evaluation.Experiment{
		ID:          doc.ID,
		Name:        doc.Name,
		Description: doc.Description,
		Variants:    doc.Variants,
		Metrics:     doc.Metrics,
		StartTime:   doc.StartTime,
		EndTime:     doc.EndTime,
		Status:      doc.Status,
	}
}

// Compile-time interface check.
var _ evaluation.ExperimentStore = (*MongoExperimentStore)(nil)
