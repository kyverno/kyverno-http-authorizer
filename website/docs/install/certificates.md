# Certificates management

The Kyverno HTTP Authorizer components (control plane and sidecar injector) come with webhooks and need valid certificates to let the API server call into them:

- **Control plane**: Has a validating webhook for ValidatingPolicy resources
- **Sidecar injector**: Has a mutating webhook to inject sidecars into pods

At deployment time you can either provide your own certificates or use [cert-manager](https://cert-manager.io) to create them.

## Bring your own

If you want to bring your own certificates, you can set `certificates.static` values when installing the helm charts.

### Control plane certificate

```bash
# create certificate for control plane
openssl req -new -x509  \
  -subj "/CN=kyverno-http-authorizer-control-plane-validation-webhook.kyverno.svc" \
  -addext "subjectAltName = DNS:kyverno-http-authorizer-control-plane-validation-webhook.kyverno.svc" \
  -nodes -newkey rsa:4096 -keyout control-plane-tls.key -out control-plane-tls.crt

# install control plane with static certificate
helm install kyverno-http-authorizer-control-plane \
  --namespace kyverno --create-namespace \
  --wait \
  --repo https://kyverno.github.io/kyverno-http-authorizer kyverno-http-authorizer-control-plane \
  --set-file certificates.static.crt=control-plane-tls.crt \
  --set-file certificates.static.key=control-plane-tls.key
```

### Sidecar injector certificate

```bash
# create certificate for sidecar injector
openssl req -new -x509  \
  -subj "/CN=kyverno-sidecar-injector.kyverno.svc" \
  -addext "subjectAltName = DNS:kyverno-sidecar-injector.kyverno.svc" \
  -nodes -newkey rsa:4096 -keyout sidecar-injector-tls.key -out sidecar-injector-tls.crt

# install sidecar injector with static certificate
helm install kyverno-sidecar-injector \
  --namespace kyverno \
  --wait \
  --repo https://kyverno.github.io/kyverno-http-authorizer kyverno-sidecar-injector \
  --set-file certificates.static.crt=sidecar-injector-tls.crt \
  --set-file certificates.static.key=sidecar-injector-tls.key \
  --set controlPlaneAddress=kyverno-http-authorizer-control-plane.kyverno.svc.cluster.local:9081
```

## Use cert-manager

If you don't want to manage certificates yourself you can rely on [cert-manager](https://cert-manager.io) to create them for you and inject them in the webhook configurations.

```bash
# install cert-manager
helm install cert-manager \
  --namespace cert-manager --create-namespace \
  --wait \
  --repo https://charts.jetstack.io cert-manager \
  --set crds.enabled=true

# create a certificate issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF
```

### Install control plane with cert-manager

```bash
# install control plane with managed certificate
helm install kyverno-http-authorizer-control-plane \
  --namespace kyverno --create-namespace \
  --wait \
  --repo https://kyverno.github.io/kyverno-http-authorizer kyverno-http-authorizer-control-plane \
  --set certificates.certManager.issuerRef.group=cert-manager.io \
  --set certificates.certManager.issuerRef.kind=ClusterIssuer \
  --set certificates.certManager.issuerRef.name=selfsigned-issuer
```

The cert-manager configuration is required because the control plane includes a validating webhook for ValidatingPolicy resources.

### Install sidecar injector with cert-manager

```bash
# install sidecar injector with managed certificate
helm install kyverno-sidecar-injector \
  --namespace kyverno \
  --wait \
  --repo https://kyverno.github.io/kyverno-http-authorizer kyverno-sidecar-injector \
  --set certificates.certManager.issuerRef.group=cert-manager.io \
  --set certificates.certManager.issuerRef.kind=ClusterIssuer \
  --set certificates.certManager.issuerRef.name=selfsigned-issuer \
  --set controlPlaneAddress=kyverno-http-authorizer-control-plane.kyverno.svc.cluster.local:9081
```

The `controlPlaneAddress` tells injected sidecars where to connect to fetch policies.
