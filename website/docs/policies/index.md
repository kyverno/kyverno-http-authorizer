# Policies

A Kyverno `ValidatingPolicy` is a custom [Kubernetes resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) and can be easily managed via Kubernetes APIs, GitOps workflows, and other existing tools.

## Resource Scope

A Kyverno `ValidatingPolicy` is a cluster-wide resource.

## API Group and Kind

A `ValidatingPolicy` belongs to the `policies.kyverno.io/v1alpha1` group and can only be of kind `ValidatingPolicy`.

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: demo
spec:
  # if something fails the request will be denied
  failurePolicy: Fail
  evaluation:
    mode: HTTP
  variables:
    # `force_authorized` references the 'x-force-authorized' header
    # from the HTTP request (or '' if not present)
  - name: force_authorized
    expression: object.headers.get("x-force-authorized")
    # `allowed` will be `true` if `variables.force_authorized` has the
    # value 'enabled' or 'true'
  - name: allowed
    expression: variables.force_authorized in ["enabled", "true"]
  validations:
    # make an authorization decision based on the value of `variables.allowed`
  - expression: |
      !variables.allowed
        ? http.response().status(403).withBody("Forbidden")
        : null
  - expression: |
      http.response().status(200)
```

## HTTP Authorization

The Kyverno HTTP Authorizer validates HTTP requests using policy-based rules written in CEL.

A Kyverno `ValidatingPolicy` analyzes an HTTP request (including headers, path, method, body, etc.) and returns an `http.Response` object to allow or deny the request.

## CEL language

A `ValidatingPolicy` uses the [CEL language](https://github.com/google/cel-spec) to process HTTP requests.

CEL is an expression language that's fast, portable, and safe to execute in performance-critical applications.

## Policy structure

A Kyverno `ValidatingPolicy` is made of:

- A [failure policy](./failure-policy.md)
- [Match conditions](./match-conditions.md) if needed
- Eventually some [variables](./variables.md)
- The [validation rules](./authorization-rules.md)
