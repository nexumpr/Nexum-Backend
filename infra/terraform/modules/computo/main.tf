###############################################################################
# Nexum-Backend — I-020 computo-go (implementacion local)
#
# Dos instancias Go simultaneas en dos AZ, sin IPv4 publica, sin SSH, con
# creditos CPU en modo standard (evita el modo unlimited por defecto de T4g) y
# user-data minimo que descarga el artefacto desde S3 (gateway endpoint) y
# verifica su SHA-256 antes de instalarlo.
#
# Nada de este directorio se ha aplicado. No crea IAM, ALB, AMI ni recursos.
###############################################################################

locals {
  nombre = "${var.proyecto}-${var.entorno}"
  azs    = sort(keys(var.subnet_ids))

  # El binario se publica en S3 (gateway endpoint: sin NAT ni IPv4 publica).
  artefacto_uri = var.artefacto_s3_uri
  artefacto_sha = lower(var.artefacto_sha256)
}

# AMI oficial y reproducible desde el parametro publico de SSM
# (`/aws/service/ami-amazon-linux-latest/...`): no se codifica ningun id.
data "aws_ssm_parameter" "ami_arm64" {
  name = var.ami_ssm_parameter
}

# Invariantes de seguridad y disponibilidad. Se evaluan en plan/apply; validate
# no las ejecuta, por eso las pruebas estaticas verifican que existan.
resource "terraform_data" "invariantes_computo" {
  input = "${var.instance_type}|${var.asg_capacidad}|${var.artefacto_sha256}|${var.iam_instance_profile_name}|${length(var.subnet_ids)}"

  lifecycle {
    precondition {
      condition     = var.instance_type == "t4g.micro"
      error_message = "I-020 fija t4g.micro (ARM64, 2 vCPU, 1 GiB); otro tipo exige recotizacion."
    }

    precondition {
      condition     = var.asg_capacidad == 2
      error_message = "Se exigen exactamente 2 instancias simultaneas (min = desired = max = 2)."
    }

    precondition {
      condition     = length(var.subnet_ids) == 2
      error_message = "Se requieren las dos subredes privadas de I-010 (una por AZ)."
    }

    precondition {
      condition     = can(regex("^[0-9a-f]{64}$", local.artefacto_sha))
      error_message = "artefacto_sha256 debe ser un SHA-256 hexadecimal de 64 caracteres."
    }

    precondition {
      condition     = var.iam_instance_profile_name != "" && var.iam_instance_profile_name != "PENDIENTE-I-060"
      error_message = "El instance profile real lo define I-060; no se inventa uno."
    }
  }
}

resource "aws_launch_template" "backend" {
  name_prefix   = "${local.nombre}-lt-"
  image_id      = data.aws_ssm_parameter.ami_arm64.value
  instance_type = var.instance_type

  # Sin clave SSH: el acceso es exclusivamente por SSM en ventana.
  key_name = null

  vpc_security_group_ids = [var.sg_backend_id]

  iam_instance_profile {
    name = var.iam_instance_profile_name
  }

  # Correccion critica: T4g usa unlimited por defecto y puede consumir creditos
  # facturables. Se fuerza standard de forma explicita.
  credit_specification {
    cpu_credits = "standard"
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required" # IMDSv2 obligatorio
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "disabled"
  }

  monitoring {
    enabled = false # metricas basicas de hipervisor (sin agente ni coste extra)
  }

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = var.root_volume_gb
      volume_type           = "gp3"
      encrypted             = true
      delete_on_termination = true
    }
  }

  user_data = base64encode(
    templatefile("${path.module}/user-data.sh.tftpl", {
      artefacto_uri = local.artefacto_uri
      artefacto_sha = local.artefacto_sha
      servicio      = var.servicio_nombre
    })
  )

  tag_specifications {
    resource_type = "instance"
    tags = {
      Name = "${local.nombre}-backend"
      Spec = "I-020-computo"
    }
  }

  tag_specifications {
    resource_type = "volume"
    tags = {
      Name = "${local.nombre}-backend-root"
    }
  }

  tags = {
    Name = "${local.nombre}-lt"
  }
}

resource "aws_autoscaling_group" "backend" {
  name                = "${local.nombre}-asg-backend"
  min_size            = var.asg_capacidad
  desired_capacity    = var.asg_capacidad
  max_size            = var.asg_capacidad
  vpc_zone_identifier = [for az in local.azs : var.subnet_ids[az]]

  # En I-030 se cambia a ELB cuando exista target group.
  health_check_type         = var.health_check_type
  health_check_grace_period = var.health_check_grace_period

  launch_template {
    id      = aws_launch_template.backend.id
    version = "$Latest"
  }

  # AWS equilibra por zona, pero no garantiza una instancia por AZ si falta
  # capacidad. "balanced-only" evita lanzar en una AZ que desequilibre.
  availability_zone_distribution {
    capacity_distribution_strategy = "balanced-only"
  }

  protect_from_scale_in = true

  instance_refresh {
    strategy = "Rolling"

    preferences {
      min_healthy_percentage = 50
      instance_warmup        = 120
    }
  }

  dynamic "tag" {
    for_each = {
      Name = "${local.nombre}-backend"
      Spec = "I-020-computo"
    }

    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}
