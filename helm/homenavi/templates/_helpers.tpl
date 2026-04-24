{{- define "homenavi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "homenavi.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.labels" -}}
helm.sh/chart: {{ include "homenavi.chart" . }}
app.kubernetes.io/name: {{ include "homenavi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "homenavi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "homenavi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "homenavi.componentName" -}}
{{- printf "%s-%s" (include "homenavi.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.componentLabels" -}}
{{ include "homenavi.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "homenavi.stableLabels" -}}
app.kubernetes.io/name: {{ include "homenavi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "homenavi.componentStableLabels" -}}
{{ include "homenavi.stableLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "homenavi.immutableComponentLabels" -}}
{{ include "homenavi.componentStableLabels" (dict "root" . "component" "redis") }}
{{- end -}}

{{- define "homenavi.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "homenavi.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.image" -}}
{{- $repository := required "service.image.repository is required" .service.image.repository -}}
{{- $tag := default .root.Chart.AppVersion (default .root.Values.global.imageTag .service.image.tag) -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}

{{- define "homenavi.jwtSecretName" -}}
{{- default "homenavi-jwt" .Values.jwt.existingSecretName -}}
{{- end -}}

{{- define "homenavi.jwtStaticSecret" -}}
{{- and .Values.jwt.createSecret (not .Values.jwt.existingSecretName) (ne (trim .Values.jwt.publicKey) "") (ne (trim .Values.jwt.privateKey) "") -}}
{{- end -}}

{{- define "homenavi.jwtBootstrapEnabled" -}}
{{- and .Values.jwt.createSecret (not .Values.jwt.existingSecretName) .Values.jwt.bootstrap.enabled (not (eq (include "homenavi.jwtStaticSecret" .) "true")) -}}
{{- end -}}

{{- define "homenavi.mqttProvider" -}}
{{- default "emqx" .Values.dependencies.mqtt.provider -}}
{{- end -}}

{{- define "homenavi.objectStorageProvider" -}}
{{- default "minio" .Values.dependencies.objectStorage.provider -}}
{{- end -}}

{{- define "homenavi.storageType" -}}
{{- $storageType := lower (trim (default "s3" .Values.storage.type)) -}}
{{- if ne $storageType "s3" -}}
{{- fail "storage.type must be s3" -}}
{{- end -}}
{{- $storageType -}}
{{- end -}}

{{- define "homenavi.storageManagedSecretName" -}}
{{- printf "%s-storage-auth" (include "homenavi.fullname" .) -}}
{{- end -}}

{{- define "homenavi.storageUsesExistingSecret" -}}
{{- ne (trim (default "" .Values.storage.s3.existingSecretName)) "" -}}
{{- end -}}

{{- define "homenavi.storageUsesManagedSecret" -}}
{{- and (eq (include "homenavi.objectStorageProvider" .) "minio") (not (eq (include "homenavi.storageUsesExistingSecret" .) "true")) -}}
{{- end -}}

{{- define "homenavi.storageS3Endpoint" -}}
{{- $override := trim (default "" .Values.storage.s3.endpoint) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else if eq (include "homenavi.objectStorageProvider" .) "minio" -}}
http://minio:9000
{{- else -}}
{{- fail "storage.s3.endpoint is required when dependencies.objectStorage.provider is not minio" -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.storageS3Region" -}}
{{- default "us-east-1" .Values.storage.s3.region -}}
{{- end -}}

{{- define "homenavi.storageS3Bucket" -}}
{{- default "profile-pictures" .Values.storage.s3.bucket -}}
{{- end -}}

{{- define "homenavi.storageS3AuthSecretName" -}}
{{- if eq (include "homenavi.storageUsesExistingSecret" .) "true" -}}
{{- trim .Values.storage.s3.existingSecretName -}}
{{- else if eq (include "homenavi.storageUsesManagedSecret" .) "true" -}}
{{- include "homenavi.storageManagedSecretName" . -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.storageS3AccessKey" -}}
{{- if ne (trim (include "homenavi.storageS3AuthSecretName" .)) "" -}}
{{- "" -}}
{{- else -}}
{{- $value := trim (default "" .Values.storage.s3.accessKey) -}}
{{- if ne $value "" -}}
{{- $value -}}
{{- else -}}
{{- fail "storage.s3.accessKey or storage.s3.existingSecretName is required when dependencies.objectStorage.provider is not minio" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.storageS3SecretKey" -}}
{{- if ne (trim (include "homenavi.storageS3AuthSecretName" .)) "" -}}
{{- "" -}}
{{- else -}}
{{- $value := trim (default "" .Values.storage.s3.secretKey) -}}
{{- if ne $value "" -}}
{{- $value -}}
{{- else -}}
{{- fail "storage.s3.secretKey or storage.s3.existingSecretName is required when dependencies.objectStorage.provider is not minio" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.storageS3ForcePathStyle" -}}
{{- printf "%v" (default true .Values.storage.s3.forcePathStyle) -}}
{{- end -}}

{{- define "homenavi.storagePresignExpirySeconds" -}}
{{- printf "%v" (default 900 .Values.storage.s3.presignExpirySeconds) -}}
{{- end -}}

{{- define "homenavi.storageS3AccessKeyKey" -}}
{{- default "accessKey" .Values.storage.s3.accessKeyKey -}}
{{- end -}}

{{- define "homenavi.storageS3SecretKeyKey" -}}
{{- default "secretKey" .Values.storage.s3.secretKeyKey -}}
{{- end -}}

{{- define "homenavi.postgresProvider" -}}
{{- default "bundled" .Values.dependencies.postgres.provider -}}
{{- end -}}

{{- define "homenavi.redisProvider" -}}
{{- default "bundled" .Values.dependencies.redis.provider -}}
{{- end -}}

{{- define "homenavi.postgresMode" -}}
{{- default "cnpgManaged" .Values.postgres.mode -}}
{{- end -}}

{{- define "homenavi.redisMode" -}}
{{- default "sentinel" .Values.redis.mode -}}
{{- end -}}

{{- define "homenavi.emqxConfigMapName" -}}
{{- printf "%s-emqx-config" (include "homenavi.fullname" .) -}}
{{- end -}}

{{- define "homenavi.zigbee2mqttConfigMapName" -}}
{{- $existing := trim (default "" .service.existingConfigurationConfigMap) -}}
{{- if ne $existing "" -}}
{{- $existing -}}
{{- else -}}
{{- printf "%s-zigbee2mqtt-config" (include "homenavi.fullname" .root) -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.cnpgClusterName" -}}
{{- default (printf "%s-postgres" (include "homenavi.fullname" .)) .Values.postgres.cnpg.clusterName -}}
{{- end -}}

{{- define "homenavi.postgresHost" -}}
{{- $override := trim (default "" .Values.postgres.host) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else if and (eq (include "homenavi.postgresProvider" .) "bundled") (eq (include "homenavi.postgresMode" .) "cnpgManaged") -}}
{{ printf "%s-rw" (include "homenavi.cnpgClusterName" .) }}
{{- else -}}
postgres
{{- end -}}
{{- end -}}

{{- define "homenavi.postgresPort" -}}
{{- default "5432" .Values.postgres.port -}}
{{- end -}}

{{- define "homenavi.postgresDatabase" -}}
{{- default "users" .Values.postgres.database -}}
{{- end -}}

{{- define "homenavi.postgresUser" -}}
{{- default "user" .Values.postgres.username -}}
{{- end -}}

{{- define "homenavi.postgresPassword" -}}
{{- default "password" .Values.postgres.password -}}
{{- end -}}

{{- define "homenavi.postgresSSLMode" -}}
{{- default "disable" .Values.postgres.sslMode -}}
{{- end -}}

{{- define "homenavi.postgresAuthSecretName" -}}
{{- $existing := trim (default "" .Values.postgres.auth.existingSecretName) -}}
{{- if ne $existing "" -}}
{{- $existing -}}
{{- else if and (eq (include "homenavi.postgresProvider" .) "bundled") (eq (include "homenavi.postgresMode" .) "cnpgManaged") -}}
{{- include "homenavi.postgresAppSecretName" . -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.postgresAuthUsernameKey" -}}
{{- default "username" .Values.postgres.auth.usernameKey -}}
{{- end -}}

{{- define "homenavi.postgresAuthPasswordKey" -}}
{{- default "password" .Values.postgres.auth.passwordKey -}}
{{- end -}}

{{- define "homenavi.postgresAppSecretName" -}}
{{- printf "%s-app" (include "homenavi.cnpgClusterName" .) -}}
{{- end -}}

{{- define "homenavi.postgresSuperuserSecretName" -}}
{{- default (printf "%s-superuser" (include "homenavi.cnpgClusterName" .)) .Values.postgres.cnpg.superuserSecretName -}}
{{- end -}}

{{- define "homenavi.redisStatefulSetName" -}}
{{- printf "%s-redis" (include "homenavi.fullname" .) -}}
{{- end -}}

{{- define "homenavi.redisHeadlessServiceName" -}}
{{- printf "%s-headless" (include "homenavi.redisStatefulSetName" .) -}}
{{- end -}}

{{- define "homenavi.redisSentinelServiceName" -}}
{{- printf "%s-sentinel" (include "homenavi.redisStatefulSetName" .) -}}
{{- end -}}

{{- define "homenavi.redisStandalonePort" -}}
{{- default "6379" .Values.redis.standalone.port -}}
{{- end -}}

{{- define "homenavi.redisSentinelPort" -}}
{{- default "26379" .Values.redis.sentinel.port -}}
{{- end -}}

{{- define "homenavi.redisAddr" -}}
{{- $override := trim (default "" .Values.redis.addr) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else -}}
{{ printf "%s:%s" (include "homenavi.redisDependencyHost" .) (include "homenavi.redisDependencyPort" .) }}
{{- end -}}
{{- end -}}

{{- define "homenavi.redisMasterName" -}}
{{- default "homenavi-redis" .Values.redis.masterName -}}
{{- end -}}

{{- define "homenavi.redisAuthSecretName" -}}
{{- default "" .Values.redis.auth.existingSecretName -}}
{{- end -}}

{{- define "homenavi.redisAuthPasswordKey" -}}
{{- default "password" .Values.redis.auth.passwordKey -}}
{{- end -}}

{{- define "homenavi.redisSentinelAddrs" -}}
{{- if gt (len (default (list) .Values.redis.sentinelAddrs)) 0 -}}
{{ join "," .Values.redis.sentinelAddrs }}
{{- else -}}
{{- $root := . -}}
{{- $replicas := int (default 3 .Values.redis.sentinel.replicas) -}}
{{- $parts := list -}}
{{- range $i := until $replicas -}}
{{- $parts = append $parts (printf "%s-%d.%s:%s" (include "homenavi.redisStatefulSetName" $root) $i (include "homenavi.redisHeadlessServiceName" $root) (include "homenavi.redisSentinelPort" $root)) -}}
{{- end -}}
{{ join "," $parts }}
{{- end -}}
{{- end -}}

{{- define "homenavi.redisDependencyHost" -}}
{{- if and (eq (include "homenavi.redisProvider" .) "bundled") (eq (include "homenavi.redisMode" .) "sentinel") -}}
{{ include "homenavi.redisSentinelServiceName" . }}
{{- else -}}
redis
{{- end -}}
{{- end -}}

{{- define "homenavi.redisDependencyPort" -}}
{{- if and (eq (include "homenavi.redisProvider" .) "bundled") (eq (include "homenavi.redisMode" .) "sentinel") -}}
{{ include "homenavi.redisSentinelPort" . }}
{{- else -}}
{{ include "homenavi.redisStandalonePort" . }}
{{- end -}}
{{- end -}}

{{- define "homenavi.serviceEnabled" -}}
{{- $name := .component -}}
{{- $svc := index .root.Values.services $name -}}
{{- $enabled := default false $svc.enabled -}}
{{- $mqttProvider := include "homenavi.mqttProvider" .root -}}
{{- $storageProvider := include "homenavi.objectStorageProvider" .root -}}
{{- $postgresProvider := include "homenavi.postgresProvider" .root -}}
{{- $postgresMode := include "homenavi.postgresMode" .root -}}
{{- $redisProvider := include "homenavi.redisProvider" .root -}}
{{- $redisMode := include "homenavi.redisMode" .root -}}
{{- if eq $name "emqx" -}}
{{- and $enabled (eq $mqttProvider "emqx") -}}
{{- else if eq $name "minio" -}}
{{- and $enabled (eq $storageProvider "minio") -}}
{{- else if eq $name "postgres" -}}
{{- and $enabled (eq $postgresProvider "bundled") (eq $postgresMode "standalone") -}}
{{- else if eq $name "redis" -}}
{{- and $enabled (eq $redisProvider "bundled") (eq $redisMode "standalone") -}}
{{- else -}}
{{- $enabled -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.mqttBrokerURL" -}}
{{- $override := trim (default "" .Values.mqtt.brokerUrl) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else -}}
mqtt://emqx:1883
{{- end -}}
{{- end -}}

{{- define "homenavi.mqttWebsocketUpstreamURL" -}}
{{- $override := trim (default "" .Values.mqtt.websocketUpstreamUrl) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else -}}
ws://emqx:8083/mqtt
{{- end -}}
{{- end -}}

{{- define "homenavi.mqttDependencyHost" -}}
{{- $override := trim (default "" .Values.mqtt.dependencyHost) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else if eq (include "homenavi.mqttProvider" .) "emqx" -}}
emqx
{{- end -}}
{{- end -}}

{{- define "homenavi.mqttDependencyPort" -}}
{{- $override := trim (default "" .Values.mqtt.dependencyPort) -}}
{{- if ne $override "" -}}
{{- $override -}}
{{- else -}}
1883
{{- end -}}
{{- end -}}
