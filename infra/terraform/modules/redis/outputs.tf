output "replication_group_id" {
  description = "ID del grupo de replicación (un único nodo compartido)"
  value       = aws_elasticache_replication_group.redis.replication_group_id
}

output "endpoint_primario" {
  description = "Endpoint que consumen ambas instancias Go (TLS obligatorio)"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "puerto" {
  description = "Puerto de Redis (6379, permitido por el SG de I-010)"
  value       = aws_elasticache_replication_group.redis.port
}

output "motor" {
  description = "Motor y versión; nunca Valkey sin ADR"
  value = {
    motor   = aws_elasticache_replication_group.redis.engine
    version = aws_elasticache_replication_group.redis.engine_version
    nodo    = aws_elasticache_replication_group.redis.node_type
  }
}

output "topologia" {
  description = "Topología real: 1 nodo, sin réplica ni failover (no es alta disponibilidad)"
  value = {
    nodos               = aws_elasticache_replication_group.redis.num_cache_clusters
    failover            = aws_elasticache_replication_group.redis.automatic_failover_enabled
    multi_az            = aws_elasticache_replication_group.redis.multi_az_enabled
    alta_disponibilidad = false
  }
}

output "seguridad" {
  description = "TLS, cifrado en reposo y usuario RBAC dedicado"
  value = {
    tls                 = aws_elasticache_replication_group.redis.transit_encryption_enabled
    cifrado_reposo      = aws_elasticache_replication_group.redis.at_rest_encryption_enabled
    user_group          = aws_elasticache_replication_group.redis.user_group_ids
    password_versionada = false
  }
}

output "cache" {
  description = "Política de evicción y retención de backups"
  value = {
    parameter_group    = aws_elasticache_parameter_group.redis.name
    maxmemory_policy   = "volatile-lru"
    snapshot_retencion = aws_elasticache_replication_group.redis.snapshot_retention_limit
  }
}

output "red" {
  description = "Subnet group privado y SG de I-010 reutilizado"
  value = {
    subnet_group = aws_elasticache_subnet_group.redis.name
    subredes     = length(aws_elasticache_subnet_group.redis.subnet_ids)
  }
}

output "estado_despliegue" {
  description = "I-050 se implementa localmente; ningún recurso AWS ha sido creado"
  value       = "NOT_RUN_AWS"
}
