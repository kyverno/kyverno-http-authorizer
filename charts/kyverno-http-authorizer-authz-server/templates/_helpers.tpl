{{/* vim: set filetype=mustache: */}}

{{- define "kyverno-http-authorizer-authz-server.name" -}}
{{ template "kyverno.lib.names.name" . }}
{{- end -}}

{{- define "kyverno-http-authorizer-authz-server.labels" -}}
{{- template "kyverno.lib.labels.merge" (list
  (include "kyverno.lib.labels.common" .)
  (include "kyverno-http-authorizer-authz-server.labels.selector" .)
) -}}
{{- end -}}

{{- define "kyverno-http-authorizer-authz-server.labels.selector" -}}
{{- template "kyverno.lib.labels.merge" (list
  (include "kyverno.lib.labels.common.selector" .)
  (include "kyverno.lib.labels.component" "authz-server")
) -}}
{{- end -}}

{{- define "kyverno-http-authorizer-authz-server.service-account.name" -}}
{{- if .Values.rbac.create -}}
  {{- default (include "kyverno-http-authorizer-authz-server.name" .) .Values.rbac.serviceAccount.name -}}
{{- else -}}
  {{- required "A service account name is required when `rbac.create` is set to `false`" .Values.rbac.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "kyverno-http-authorizer-authz-server.image" -}}
{{- printf "%s/%s:%s" .registry .repository (default "latest" .tag) -}}
{{- end -}}
