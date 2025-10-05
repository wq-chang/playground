terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
  required_version = "~> 1.12"
}

# Configure the AWS Provider
provider "aws" {
  region = "us-east-1"
  endpoints {
    apigateway = "http://localhost:4566"
  }
}
