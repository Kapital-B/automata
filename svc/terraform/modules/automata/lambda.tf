resource "aws_iam_role" "api" {
  name = "automata-api-role-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = "sts:AssumeRole"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role" "scheduler" {
  name = "automata-scheduler-role-${var.environment}"

  assume_role_policy = aws_iam_role.api.assume_role_policy
  tags               = local.common_tags
}

resource "aws_iam_role" "worker" {
  name = "automata-worker-role-${var.environment}"

  assume_role_policy = aws_iam_role.api.assume_role_policy
  tags               = local.common_tags
}

resource "aws_iam_role" "migrate" {
  name = "automata-migrate-role-${var.environment}"

  assume_role_policy = aws_iam_role.api.assume_role_policy
  tags               = local.common_tags
}

resource "aws_iam_role_policy_attachment" "api_basic_execution" {
  role       = aws_iam_role.api.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "scheduler_basic_execution" {
  role       = aws_iam_role.scheduler.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "worker_basic_execution" {
  role       = aws_iam_role.worker.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "migrate_basic_execution" {
  role       = aws_iam_role.migrate.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "api" {
  name = "automata-api-policy-${var.environment}"
  role = aws_iam_role.api.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [
        {
          Effect = "Allow"
          Action = [
            "dynamodb:DeleteItem",
            "dynamodb:GetItem",
            "dynamodb:PutItem",
            "dynamodb:Query",
            "dynamodb:TransactWriteItems",
            "dynamodb:UpdateItem"
          ]
          Resource = [
            aws_dynamodb_table.jobs.arn,
            "${aws_dynamodb_table.jobs.arn}/index/*"
          ]
        }
      ],
      local.enable_hosted ? concat(
        [
          {
            Effect   = "Allow"
            Action   = ["dsql:DbConnect"]
            Resource = [aws_dsql_cluster.hosted[0].arn]
          },
          {
            Effect   = "Allow"
            Action   = ["secretsmanager:GetSecretValue"]
            Resource = local.hosted_secret_arns
          },
        ],
        local.bedrock_invoke_statements,
      ) : []
    )
  })
}

resource "aws_iam_role_policy" "scheduler" {
  name = "automata-scheduler-policy-${var.environment}"
  role = aws_iam_role.scheduler.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [
        {
          Effect = "Allow"
          Action = [
            "dynamodb:DeleteItem",
            "dynamodb:GetItem",
            "dynamodb:PutItem",
            "dynamodb:Query",
            "dynamodb:TransactWriteItems",
            "dynamodb:UpdateItem"
          ]
          Resource = [
            aws_dynamodb_table.jobs.arn,
            "${aws_dynamodb_table.jobs.arn}/index/*"
          ]
        }
      ],
      local.enable_hosted ? [
        {
          Effect   = "Allow"
          Action   = ["dsql:DbConnect"]
          Resource = [aws_dsql_cluster.hosted[0].arn]
        },
        {
          Effect   = "Allow"
          Action   = ["secretsmanager:GetSecretValue"]
          Resource = local.hosted_secret_arns
        }
      ] : []
    )
  })
}

resource "aws_iam_role_policy" "worker" {
  name = "automata-worker-policy-${var.environment}"
  role = aws_iam_role.worker.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [
        {
          Effect = "Allow"
          Action = [
            "dynamodb:DeleteItem",
            "dynamodb:DescribeStream",
            "dynamodb:GetItem",
            "dynamodb:GetRecords",
            "dynamodb:GetShardIterator",
            "dynamodb:ListStreams",
            "dynamodb:PutItem",
            "dynamodb:Query",
            "dynamodb:TransactWriteItems",
            "dynamodb:UpdateItem"
          ]
          Resource = [
            aws_dynamodb_table.jobs.arn,
            "${aws_dynamodb_table.jobs.arn}/index/*",
            "${aws_dynamodb_table.jobs.arn}/stream/*"
          ]
        },
        {
          Effect = "Allow"
          Action = [
            "s3:GetObject",
            "s3:ListBucket",
            "s3:PutObject"
          ]
          Resource = [
            aws_s3_bucket.stream_failures.arn,
            "${aws_s3_bucket.stream_failures.arn}/*"
          ]
        }
      ],
      local.enable_hosted ? concat(
        [
          {
            Effect   = "Allow"
            Action   = ["dsql:DbConnect"]
            Resource = [aws_dsql_cluster.hosted[0].arn]
          },
          {
            Effect   = "Allow"
            Action   = ["secretsmanager:GetSecretValue"]
            Resource = local.hosted_secret_arns
          },
        ],
        local.bedrock_invoke_statements,
      ) : []
    )
  })
}

resource "aws_iam_role_policy" "migrate" {
  name = "automata-migrate-policy-${var.environment}"
  role = aws_iam_role.migrate.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = local.enable_hosted ? [
      {
        Effect   = "Allow"
        Action   = ["dsql:DbConnectAdmin"]
        Resource = [aws_dsql_cluster.hosted[0].arn]
      }
    ] : []
  })
}

locals {
  bootstrap_fingerprint = sha256(jsonencode({
    api_main        = filesha256("${path.module}/../../../cmd/api/main.go")
    scheduler_main  = filesha256("${path.module}/../../../cmd/scheduler/main.go")
    worker_main     = filesha256("${path.module}/../../../cmd/worker/main.go")
    migrate_main    = filesha256("${path.module}/../../../cmd/migrate/main.go")
    migrate_pkg     = filesha256("${path.module}/../../../internal/adapters/outbound/persistence/migrate/migrate.go")
    migrate_runtime = filesha256("${path.module}/../../../internal/adapters/outbound/persistence/migrate/dsql_runtime.go")
    migrate_common  = filesha256("${path.module}/../../../internal/adapters/outbound/persistence/migrate/common/001_baseline.sql")
    composition     = filesha256("${path.module}/../../../internal/composition/app.go")
    factory         = filesha256("${path.module}/../../../internal/adapters/outbound/persistence/factory/factory.go")
    factory_dsql    = filesha256("${path.module}/../../../internal/adapters/outbound/persistence/factory/dsql.go")
    execution       = filesha256("${path.module}/../../../internal/application/jobs/execution.go")
    scheduler_svc   = filesha256("${path.module}/../../../internal/application/jobs/scheduler.go")
    dynamodb_store  = filesha256("${path.module}/../../../internal/adapters/outbound/dynamodbjobs/store.go")
    go_mod          = filesha256("${path.module}/../../../go.mod")
    go_sum          = filesha256("${path.module}/../../../go.sum")
    arch            = local.lambda_go_arch
  }))
}

# Build during plan/refresh so archive_file source dirs exist on clean checkouts.
data "external" "build_bootstraps" {
  program = ["bash", "${path.module}/build_bootstraps.sh"]
  query = {
    arch        = local.lambda_go_arch
    fingerprint = local.bootstrap_fingerprint
  }
}

data "archive_file" "api" {
  type        = "zip"
  source_dir  = "${path.module}/.dist/${data.external.build_bootstraps.result.api}"
  output_path = "${path.module}/api-bootstrap.zip"
}

data "archive_file" "scheduler" {
  type        = "zip"
  source_dir  = "${path.module}/.dist/${data.external.build_bootstraps.result.scheduler}"
  output_path = "${path.module}/scheduler-bootstrap.zip"
}

data "archive_file" "worker" {
  type        = "zip"
  source_dir  = "${path.module}/.dist/${data.external.build_bootstraps.result.worker}"
  output_path = "${path.module}/worker-bootstrap.zip"
}

data "archive_file" "migrate" {
  type        = "zip"
  source_dir  = "${path.module}/.dist/${data.external.build_bootstraps.result.migrate}"
  output_path = "${path.module}/migrate-bootstrap.zip"
}

resource "aws_lambda_function" "api" {
  filename         = data.archive_file.api.output_path
  function_name    = "automata-api-${var.environment}"
  role             = aws_iam_role.api.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_aws_architecture]
  source_code_hash = data.archive_file.api.output_base64sha256
  timeout          = 30
  memory_size      = 1024

  environment {
    variables = local.lambda_env
  }

  tags = local.common_tags
}

resource "aws_lambda_function" "scheduler" {
  filename         = data.archive_file.scheduler.output_path
  function_name    = "automata-scheduler-${var.environment}"
  role             = aws_iam_role.scheduler.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_aws_architecture]
  source_code_hash = data.archive_file.scheduler.output_base64sha256
  timeout          = 300
  memory_size      = 512

  environment {
    variables = local.lambda_env
  }

  tags = local.common_tags
}

resource "aws_lambda_function" "worker" {
  filename         = data.archive_file.worker.output_path
  function_name    = "automata-worker-${var.environment}"
  role             = aws_iam_role.worker.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_aws_architecture]
  source_code_hash = data.archive_file.worker.output_base64sha256
  timeout          = 900
  memory_size      = 1024

  environment {
    variables = local.lambda_env
  }

  tags = local.common_tags
}

resource "aws_lambda_function" "migrate" {
  filename         = data.archive_file.migrate.output_path
  function_name    = "automata-migrate-${var.environment}"
  role             = aws_iam_role.migrate.arn
  handler          = "bootstrap"
  runtime          = "provided.al2023"
  architectures    = [local.lambda_aws_architecture]
  source_code_hash = data.archive_file.migrate.output_base64sha256
  publish          = local.enable_hosted
  timeout          = 900
  memory_size      = 512

  environment {
    variables = local.migrate_lambda_env
  }

  tags = local.common_tags
}

resource "aws_lambda_invocation" "run_migrations" {
  count = var.run_migrations_on_deploy ? 1 : 0

  function_name = aws_lambda_function.migrate.function_name
  input = jsonencode({
    deployment_sha = data.archive_file.migrate.output_base64sha256
    migration_sha = sha1(join("", [
      for filename in sort(fileset("${path.module}/../../../internal/adapters/outbound/persistence/migrate", "**/*.sql")) :
      filesha256("${path.module}/../../../internal/adapters/outbound/persistence/migrate/${filename}")
    ]))
    runtime_iam_arns = lookup(local.migrate_lambda_env, "DSQL_RUNTIME_IAM_ROLE_ARNS", "")
  })

  triggers = {
    deployment_sha = data.archive_file.migrate.output_base64sha256
    migration_sha = sha1(join("", [
      for filename in sort(fileset("${path.module}/../../../internal/adapters/outbound/persistence/migrate", "**/*.sql")) :
      filesha256("${path.module}/../../../internal/adapters/outbound/persistence/migrate/${filename}")
    ]))
    runtime_iam_arns = lookup(local.migrate_lambda_env, "DSQL_RUNTIME_IAM_ROLE_ARNS", "")
  }

  depends_on = [
    aws_lambda_function.migrate,
    aws_cloudwatch_log_group.migrate,
    aws_iam_role_policy.api,
    aws_iam_role_policy.scheduler,
    aws_iam_role_policy.worker,
  ]
}

resource "aws_lambda_event_source_mapping" "jobs_stream" {
  event_source_arn              = aws_dynamodb_table.jobs.stream_arn
  function_name                 = aws_lambda_function.worker.arn
  starting_position             = "TRIM_HORIZON"
  batch_size                    = 1
  maximum_retry_attempts        = 3
  maximum_record_age_in_seconds = 3600
  function_response_types       = ["ReportBatchItemFailures"]

  destination_config {
    on_failure {
      destination_arn = aws_s3_bucket.stream_failures.arn
    }
  }
}
