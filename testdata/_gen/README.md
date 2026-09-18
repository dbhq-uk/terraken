# How the replacement-ordering fixtures were made

The three plan files these roots produce are the evidence behind a claim the
tool makes in prose, so the roots are committed rather than described. A
fixture that proves something about Terraform's behaviour is only evidence if
somebody else can regenerate it.

`terraform_data` and the `local` provider only, never real infrastructure -
see `AGENTS.md`. Nothing here needs a credential or a network.

Generated with **Terraform v1.16.1 on linux_amd64**.

    cd <root>
    terraform init
    terraform apply -auto-approve -var release=v1
    terraform plan -out=tfplan -var release=v2
    terraform show -json tfplan | python3 -m json.tool --indent 2 > ../../<name>.json

The `id` attribute differs between runs, so a regenerated file will not be
byte-identical to the committed one. Nothing asserts that it is.

## What each root is for

| Root | Shows |
|---|---|
| `replace-destroy-first` | the default replacement: `["delete", "create"]` |
| `replace-create-first` | the same root plus `lifecycle { create_before_destroy = true }`, which plans `["create", "delete"]` |
| `replace-create-first-propagated` | the rule set on `downstream` and NOT on `upstream`, and both planned `["create", "delete"]` |

The first two roots are identical apart from the `lifecycle` block, and the
plan files they produce are identical apart from the order of one array and the
random `id`. That is the whole evidence for "the order of the actions array is
the only signal": the block itself appears nowhere in either file.

The third root is the evidence for the limit on that. `create_before_destroy`
propagates down the dependency chain, so terraken can read the ORDER off a plan
and can never read the CAUSE - `upstream` sets no lifecycle block at all and is
still planned create-first, because `downstream` depends on it and does.
