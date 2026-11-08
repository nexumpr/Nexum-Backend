# Fixture deliberadamente inseguro para la prueba negativa del escáner IaC (I-080).
# NO forma parte de infra/terraform ni se despliega: sólo lo analiza Trivy en CI
# para demostrar que un hallazgo HIGH/CRITICAL hace fallar el job.
terraform {
  required_version = ">= 1.10.0"
}

resource "aws_s3_bucket" "inseguro" {
  bucket = "nexum-fixture-inseguro-i080"
}

resource "aws_s3_bucket_acl" "inseguro" {
  bucket = aws_s3_bucket.inseguro.id
  acl    = "public-read"
}

resource "aws_s3_bucket_public_access_block" "inseguro" {
  bucket                  = aws_s3_bucket.inseguro.id
  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}
