# Failure policy

FailurePolicy defines how to handle failures for the policy.

Failures can occur from CEL expression parse errors, type check errors, runtime errors and invalid or mis-configured policy definitions.

Allowed values are:

- `Ignore`
- `Fail`

If not set, the failure policy defaults to `Fail`.

!!!info

    FailurePolicy does not define how validations that return `null` or specific HTTP responses are handled. It only controls behavior when errors occur during policy evaluation.

## Fail

When set to `Fail`, any errors during policy evaluation will cause the request to be denied.

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
  - name: force_authorized
    expression: object.headers.get("x-force-authorized")
  - name: allowed
    expression: variables.force_authorized in ["enabled", "true"]
  validations:
  - expression: |
      !variables.allowed
        ? http.response().status(403).withBody("Forbidden")
        : null
  - expression: |
      http.response().status(200)
```

## Ignore

When set to `Ignore`, errors during policy evaluation will be ignored and the request will be allowed.

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: demo
spec:
  # if something fails the failure will be ignored and the request will be allowed
  failurePolicy: Ignore
  evaluation:
    mode: HTTP
  variables:
  - name: force_authorized
    expression: object.headers.get("x-force-authorized")
  - name: allowed
    expression: variables.force_authorized in ["enabled", "true"]
  validations:
  - expression: |
      !variables.allowed
        ? http.response().status(403).withBody("Forbidden")
        : null
  - expression: |
      http.response().status(200)
```
