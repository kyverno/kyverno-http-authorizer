# Variables

A Kyverno `ValidatingPolicy` can define `variables` that will be made available to all validation rules.

Variables can be used in composition of other expressions.
Each variable is defined as a named [CEL](https://github.com/google/cel-spec) expression.
They will be available under `variables` in other expressions of the policy.

The expression of a variable can refer to other variables defined earlier in the list but not those after. Thus, variables must be sorted by the order of first appearance and acyclic.

!!!info

    The incoming HTTP request is made available to the policy under the `object` identifier.

## Example

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: demo
spec:
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

## Using variables with external data

Variables can also fetch data from external sources:

```yaml
apiVersion: policies.kyverno.io/v1alpha1
kind: ValidatingPolicy
metadata:
  name: external-data-policy
spec:
  evaluation:
    mode: HTTP
  variables:
  - name: secretWord
    expression: |
      http.Get("http://my-server:3000").secretWord
  - name: userSecret
    expression: |
      object.headers.get("secret-header")
  - name: isValid
    expression: |
      variables.userSecret == variables.secretWord
  validations:
  - expression: |
      variables.isValid
        ? http.response().status(200).withBody("Valid secret")
        : http.response().status(403).withBody("Invalid secret")
```

In this example:
- `secretWord` fetches data from an external HTTP service
- `userSecret` extracts a header from the request
- `isValid` compares the two values
- The validation uses `variables.isValid` to make the authorization decision
