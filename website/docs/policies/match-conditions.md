# Match conditions

You can define match conditions if you need fine-grained request filtering.

Match conditions are **CEL expressions**. All match conditions must evaluate to `true` for the policy to be evaluated.

!!!info

    The policy [variables](./variables.md) will NOT be available in match conditions because they are evaluated before the rest of the policy.

## Example: Host matching

A common use case is to match requests based on the host header:

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: myapp-policy
spec:
  failurePolicy: Fail
  evaluation:
    mode: HTTP
  matchConditions:
  - name: host
    expression: |
      object.host == "myapp.com"
  - name: api-path
    expression: |
      object.path.startsWith("/api/v1")
  validations:
  - expression: |
      object.method == "GET"
        ? http.response().status(200)
        : http.response().status(405).withBody("Method not allowed")
```

In the policy above:

- The policy only applies to requests where the host is `myapp.com` AND the path starts with `/api/v1`
- If an incoming request has a different host or path, the match conditions return `false` and the policy won't apply
- If both match conditions are `true`, the validations are evaluated

## Example: Header matching

You can also match based on request headers:

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: header-check-policy
spec:
  failurePolicy: Fail
  evaluation:
    mode: HTTP
  matchConditions:
  - name: has-auth-header
    expression: |
      object.headers.get("authorization") != ""
  validations:
  - expression: |
      object.headers.get("authorization").startsWith("Bearer ")
        ? http.response().status(200)
        : http.response().status(401).withBody("Invalid authorization header")
```

## Error handling

In the event of an error evaluating a match condition the policy is not evaluated. Whether to reject the request is determined as follows:

1. If any match condition evaluated to `false` (regardless of other errors), then the policy is skipped.
1. Otherwise:
    - for `failurePolicy: Fail`, reject the request (without evaluating the policy).
    - for `failurePolicy: Ignore`, proceed with the request but skip the policy.
