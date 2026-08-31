package dynamodbjobs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/dynamodbjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/jobstoretest"
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
