resource "aws_iam_role" "api_auth_verifier_lambda_role" {
  name = "api-auth-verifier-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Action = "sts:AssumeRole",
        Effect = "Allow",
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

# Lambda function
resource "aws_lambda_function" "api_auth_verifier_lambda" {
  function_name = "api-auth-verifier"
  role          = aws_iam_role.api_auth_verifier_lambda_role.arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  filename      = "../../lambda-handler.zip"
  environment {
    variables = {
      KEYCLOAK_URL   = "http://keycloak:8080"
      KEYCLOAK_REALM = "playground"
    }
  }

  source_code_hash = filebase64sha256("../../lambda-handler.zip")
}

