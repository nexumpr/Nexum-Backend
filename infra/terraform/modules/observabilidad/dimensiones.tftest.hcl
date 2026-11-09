mock_provider "aws" {}

variables {
  entorno          = "test"
  alb_arn          = "arn:aws:elasticloadbalancing:us-east-2:123456789012:loadbalancer/app/nexum-alb/abc123"
  target_group_arn = "arn:aws:elasticloadbalancing:us-east-2:123456789012:targetgroup/nexum-tg/def456"
}

run "dimensiones_alb_y_tg" {
  command = plan

  assert {
    condition     = local.alb_suffix == "app/nexum-alb/abc123"
    error_message = "LoadBalancer debe ser app/<nombre>/<id>, sin prefijo loadbalancer/: ${local.alb_suffix}"
  }

  assert {
    condition     = local.tg_suffix == "targetgroup/nexum-tg/def456"
    error_message = "TargetGroup debe ser targetgroup/<nombre>/<id>: ${local.tg_suffix}"
  }
}
