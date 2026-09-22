package dynamodbjobs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/dynamodbjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/jobstoretest"
	"github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

func TestJobStoreContract(t *testing.T) {
	if os.Getenv("AUTOMATA_TEST_DDB_ENDPOINT") == "" {
		t.Skip("AUTOMATA_TEST_DDB_ENDPOINT not set")
	}
	jobstoretest.RunContractTests(t, newDynamoFactory(t))
}

func TestJobStoreCrashWindows(t *testing.T) {
	if os.Getenv("AUTOMATA_TEST_DDB_ENDPOINT") == "" {
		t.Skip("AUTOMATA_TEST_DDB_ENDPOINT not set")
	}
	jobstoretest.RunCrashWindowTests(t, newDynamoFactory(t))
}

func newDynamoFactory(t *testing.T) jobstoretest.Factory {
	t.Helper()
	endpoint := os.Getenv("AUTOMATA_TEST_DDB_ENDPOINT")
	return func(t *testing.T) (driven.JobStore, func()) {
		t.Helper()
		client := newDynamoClient(t, endpoint)
		tableName := "automata-jobs-test-" + uuid.NewString()
		createTestTable(t, client, tableName)
		return dynamodbjobs.NewStore(client, tableName, []byte("ddb-test-cursor-key")), func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, _ = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
		}
	}
}

func newDynamoClient(t *testing.T, endpoint string) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "test")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

func createTestTable(t *testing.T, client *dynamodb.Client, tableName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(tableName),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi1pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi1sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi2pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi2sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi3pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi3sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi4pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi4sk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi5pk"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("gsi5sk"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("gsi1"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi1pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("gsi1sk"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String("gsi2"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi2pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("gsi2sk"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String("gsi3"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi3pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("gsi3sk"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String("gsi4"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi4pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("gsi4sk"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
			{
				IndexName: aws.String("gsi5"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("gsi5pk"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("gsi5sk"), KeyType: types.KeyTypeRange},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 60*time.Second); err != nil {
		t.Fatal(err)
	}
}

// A chain's lock is transferred from the step that acquired it to the step
// that follows. The terminal paths release a lock only when the job itself
// records the scope and key, so a hand-off that forgets to stamp them leaves
// the lock behind on success: every completed chain orphans its lock, and the
// scope only recovers because a later caller reclaims it from a dead owner.
func TestCompletedChainDeletesItsLockRow(t *testing.T) {
	endpoint := os.Getenv("AUTOMATA_TEST_DDB_ENDPOINT")
	if endpoint == "" {
		t.Skip("AUTOMATA_TEST_DDB_ENDPOINT not set")
	}
	ctx := context.Background()
	client := newDynamoClient(t, endpoint)
	tableName := "automata-jobs-test-" + uuid.NewString()
	createTestTable(t, client, tableName)
	defer func() {
		_, _ = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	}()
	store := dynamodbjobs.NewStore(client, tableName, []byte("ddb-test-cursor-key"))

	now := time.Now().UTC()
	user := uuid.New()
	account := uuid.New()
	enq := &jobs.Enqueuer{Store: store}
	first, err := enq.EnqueueChain(ctx, user, &account, driven.JobTriggerAPI,
		[]string{jobs.TypeSync, jobs.TypeResolveContacts}, driven.JobPayload{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	lockKey := map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "MAILBOX#" + account.String()},
		"sk": &types.AttributeValueMemberS{Value: "LOCK#SYNC"},
	}
	held, err := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(tableName), Key: lockKey})
	if err != nil {
		t.Fatal(err)
	}
	if len(held.Item) == 0 {
		t.Fatal("expected the first step to hold the mailbox lock")
	}

	step := first
	for step != nil {
		running, err := store.KickPending(ctx, step.ID, step.Revision, "worker", now.Add(time.Minute), now)
		if err != nil {
			t.Fatal(err)
		}
		var next *driven.CreateJobInput
		if len(running.RemainingJobs) > 0 {
			next = &driven.CreateJobInput{
				ID:            jobs.DeterministicJobID(running.ChainID, running.StepIndex+1, running.RemainingJobs[0]),
				JobType:       running.RemainingJobs[0],
				UserID:        running.UserID,
				AccountID:     running.AccountID,
				TriggerKind:   running.TriggerKind,
				ChainID:       running.ChainID,
				StepIndex:     running.StepIndex + 1,
				RemainingJobs: append([]string(nil), running.RemainingJobs[1:]...),
				Payload:       running.Payload,
				Now:           now,
			}
		}
		if _, err := store.CompleteStep(ctx, running.ID, running.Revision, *running.AttemptID,
			driven.JobProgress{Processed: 1}, next, now, time.Hour); err != nil {
			t.Fatal(err)
		}
		step = nil
		if next != nil {
			got, err := store.GetByID(ctx, next.ID)
			if err != nil {
				t.Fatal(err)
			}
			step = got
		}
	}

	after, err := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(tableName), Key: lockKey})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Item) != 0 {
		t.Fatalf("the finished chain left its lock row behind: %v", after.Item)
	}
}
