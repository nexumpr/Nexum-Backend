# I-030 — Balanceo real con ALB interno y publicación HTTPS por CloudFront.
#
# Diseño ratificado D2 (`sdd/_control/H08/RFC-005-ENMIENDA_I030_TLS_D2.md`):
#   cliente --HTTPS--> CloudFront (*.cloudfront.net) --VPC Origin--> ALB interno
#   --HTTP 80 privado--> target group 8080 --> 2 instancias Go del ASG de I-020.
#
# Restricciones duras:
#  - El ALB NUNCA es internet-facing y no tiene listener 443 ni certificado ACM.
#  - No se crean certificados, dominios, Route 53, WAF ni IPv4 públicas.
#  - El security group del ALB es propiedad de I-010 (ingress 80 sólo desde la
#    prefix list administrada de CloudFront). Este módulo no lo duplica.
#  - La API no se cachea: Managed-CachingDisabled + todo el visor reenviado.
#  - Nada de esto se aplica en I-030: `terraform plan/apply` están denegados.

terraform {
  required_version = ">= 1.4.0, < 2.0.0"

  required_providers {
    aws = {
      source = "hashicorp/aws"
      # `aws_cloudfront_vpc_origin` existe desde el proveedor 5.72.0.
      version = ">= 5.72.0, < 6.0.0"
    }
  }
}

locals {
  nombre      = "${var.proyecto}-${var.entorno}"
  subredes    = [for az in sort(keys(var.subnet_ids)) : var.subnet_ids[az]]
  origin_id   = "${local.nombre}-alb-privado"
  zonas_count = length(distinct(values(var.subnet_ids)))
}

# Precondiciones evaluadas por Terraform (no por `validate`, que no las ejecuta).
resource "terraform_data" "validacion_balanceo" {
  input = {
    zonas      = local.zonas_count
    region     = var.region
    sg_alb     = var.sg_alb_id
    sg_backend = var.sg_backend_id
    health     = var.health_check_path
  }

  lifecycle {
    precondition {
      condition     = local.zonas_count == 2
      error_message = "El ALB exige exactamente 2 subredes privadas en zonas distintas (GOV-02)."
    }

    precondition {
      condition     = var.region == "us-east-2"
      error_message = "CloudFront VPC Origins e I-000/I-010 fijan us-east-2; otra región exige recotización."
    }

    precondition {
      condition     = var.sg_alb_id != var.sg_backend_id
      error_message = "El security group del ALB y el del backend deben ser distintos (mínimo privilegio)."
    }

    precondition {
      condition     = !endswith(var.health_check_path, "/ready")
      error_message = "El health check del target group no puede usar /ready: devuelve 503 por diseño hasta I-040/I-050."
    }
  }
}

# --- Balanceador interno ----------------------------------------------------

resource "aws_lb" "principal" {
  name               = "${local.nombre}-alb"
  internal           = true
  load_balancer_type = "application"
  subnets            = local.subredes
  security_groups    = [var.sg_alb_id]

  enable_http2               = true
  drop_invalid_header_fields = true
  idle_timeout               = 60
  enable_deletion_protection = false # teardown del plan Free requiere poder destruir; destroy exige Gate

  tags = merge(
    {
      Name = "${local.nombre}-alb-interno"
      Spec = "I-030-balanceo-https"
    },
    var.etiquetas
  )
}

resource "aws_lb_target_group" "backend" {
  name        = "${local.nombre}-tg-backend"
  port        = var.puerto_backend
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "instance"

  # Drenado acotado: el proceso Go cierra en <= 15 s (A-030) y systemd espera 20 s.
  deregistration_delay = var.deregistration_delay

  health_check {
    enabled             = true
    healthy_threshold   = var.healthy_threshold
    unhealthy_threshold = var.unhealthy_threshold
    interval            = var.health_check_interval
    timeout             = var.health_check_timeout
    path                = var.health_check_path
    port                = "traffic-port"
    protocol            = "HTTP"
    matcher             = "200"
  }

  # Backend stateless (A-020): sin afinidad de sesión (bloque omitido = deshabilitada).

  tags = merge(
    {
      Name = "${local.nombre}-tg-backend"
      Spec = "I-030-balanceo-https"
    },
    var.etiquetas
  )
}

resource "aws_lb_listener" "http_privado" {
  load_balancer_arn = aws_lb.principal.arn
  port              = var.puerto_alb
  protocol          = "HTTP"

  # Sin redirección 80->443 aquí: el ALB sólo recibe tráfico de CloudFront y la
  # redirección de visitantes ocurre en la distribución (D2).
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.backend.arn
  }

  tags = merge(
    {
      Name = "${local.nombre}-listener-http-privado"
      Spec = "I-030-balanceo-https"
    },
    var.etiquetas
  )
}

# Asocia los dos destinos reales del ASG sin crear servidores adicionales.
resource "aws_autoscaling_attachment" "backend" {
  autoscaling_group_name = var.asg_name
  lb_target_group_arn    = aws_lb_target_group.backend.arn
}

# --- CloudFront (borde HTTPS) ---------------------------------------------

# Requisitos oficiales verificados (AWS, 2026-11-06): IGW adjunto (I-010), subred
# privada con al menos una IPv4 libre, SG del ALB con ingress desde la prefix list
# `com.amazonaws.global.cloudfront.origin-facing` y us-east-2 soportada.
resource "aws_cloudfront_vpc_origin" "alb" {
  vpc_origin_endpoint_config {
    name                   = "${local.nombre}-vpc-origin"
    arn                    = aws_lb.principal.arn
    http_port              = var.puerto_alb
    https_port             = 443
    origin_protocol_policy = "http-only" # límite aceptado en D2: sin TLS de aplicación en el tramo privado

    origin_ssl_protocols {
      items    = ["TLSv1.2"]
      quantity = 1
    }
  }

  tags = merge(
    {
      Name = "${local.nombre}-vpc-origin"
      Spec = "I-030-balanceo-https"
    },
    var.etiquetas
  )
}

# Políticas administradas por AWS: sin caché para respuestas financieras y
# reenvío de todas las cabeceras del visor (incluida Authorization) salvo Host.
data "aws_cloudfront_cache_policy" "sin_cache" {
  name = "Managed-CachingDisabled"
}

data "aws_cloudfront_origin_request_policy" "api" {
  name = "Managed-AllViewer"
}

resource "aws_cloudfront_distribution" "api" {
  enabled         = true
  is_ipv6_enabled = var.ipv6_habilitado
  comment         = "Nexum-Backend API - I-030 (D2: CloudFront HTTPS + ALB interno)"
  price_class     = var.cloudfront_price_class
  http_version    = "http2"

  origin {
    # `DomainName` es el nombre DNS del recurso de origen (documentación CloudFront:
    # «Origin domain is the DNS domain name of the resource», ejemplo ELB
    # `*.us-west-2.elb.amazonaws.com`). El ARN del VPC origin NO es un nombre DNS y
    # el recurso `VpcOrigin` ni siquiera expone campo `DomainName`: se usa el DNS del
    # ALB interno y el vínculo con el VPC origin va en `vpc_origin_config`.
    domain_name = aws_lb.principal.dns_name
    origin_id   = local.origin_id

    connection_attempts = 3
    connection_timeout  = 10

    vpc_origin_config {
      vpc_origin_id = aws_cloudfront_vpc_origin.alb.id
    }
  }

  default_cache_behavior {
    target_origin_id = local.origin_id

    # Ningún visitante recibe HTTP: CloudFront redirige a HTTPS.
    viewer_protocol_policy = "redirect-to-https"

    # API REST completa: no se restringe ningún método ni cuerpo.
    allowed_methods = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods  = ["GET", "HEAD"]

    # Sin caché (TTL 0 gestionado por AWS) y con Authorization/idempotencia reenviados.
    cache_policy_id          = data.aws_cloudfront_cache_policy.sin_cache.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.api.id

    # Compresión desactivada: la respuesta financiera no se reescribe en el borde.
    compress = false
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    # Certificado por defecto de CloudFront: sin ACM, sin dominio propio (D2).
    # LIMITACIÓN OFICIAL: con el certificado por defecto AWS fija la security
    # policy en TLSv1 y no permite elegir TLSv1.2_2021 sin un certificado propio.
    cloudfront_default_certificate = true
  }

  tags = merge(
    {
      Name = "${local.nombre}-cloudfront-api"
      Spec = "I-030-balanceo-https"
    },
    var.etiquetas
  )
}
