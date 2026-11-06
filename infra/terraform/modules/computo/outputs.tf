output "launch_template_id" {
  description = "Launch Template del backend"
  value       = aws_launch_template.backend.id
}

output "launch_template_latest_version" {
  description = "Version mas reciente del Launch Template"
  value       = aws_launch_template.backend.latest_version
}

output "asg_name" {
  description = "Nombre del Auto Scaling Group (2 instancias, 2 AZ)"
  value       = aws_autoscaling_group.backend.name
}

output "capacidad" {
  description = "Capacidad fija de la fase actual"
  value = {
    min     = aws_autoscaling_group.backend.min_size
    desired = aws_autoscaling_group.backend.desired_capacity
    max     = aws_autoscaling_group.backend.max_size
  }
}

output "cpu_credits" {
  description = "Modo de creditos CPU aplicado (standard, nunca unlimited por defecto)"
  value       = aws_launch_template.backend.credit_specification[0].cpu_credits
}

output "puerto_backend" {
  description = "Puerto HTTP del proceso Go al que apuntara el target group de I-030"
  value       = var.puerto_backend
}

output "health_endpoints" {
  description = "Rutas de salud que consumira el target group y el ALB (I-030)"
  value = {
    liveness  = "/api/v1/health/live"
    readiness = "/api/v1/health/ready"
    puerto    = var.puerto_backend
  }
}

output "subredes_por_az" {
  description = "AZ -> subred privada usada por el ASG"
  value       = { for az in sort(keys(var.subnet_ids)) : az => var.subnet_ids[az] }
}
