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

## The sequencing roots

The same rule applies: `terraform_data` only, no credential, no network,
Terraform v1.16.1. These exist because the sequencing report states what
Terraform's apply ordering IS, and that is a claim about Terraform rather than
about the plan file - so it was run rather than cited.

| Root | Plan | Observed apply order |
|---|---|---|
| `sequence-chain` | `sequence-chain.json` | `leaf.destroy`, `middle.destroy`, `base.destroy`, `base.create`, `middle.create`, `leaf.create` |
| `sequence-update` | `sequence-update.json` | `base.destroy`, `base.create`, `follower.modify` |
| `sequence-update-with-dependant` | `sequence-update-with-dependant.json` | an update in place whose dependant is replaced, which is the shape the destructive-only guard exists for |

Three things those runs settled, none of which was obvious enough to assume:

- **The whole chain is torn down before any of it is rebuilt.** Not
  `leaf.destroy, leaf.create, middle.destroy, ...` - every destroy in the chain
  runs first, then every create. So the resource furthest from the change is
  destroyed first and created last.
- **An updated dependant is ordered after too.** The first version of the
  feature counted only creates and destroys, and left a step of the apply out
  of a report that claimed to describe it.
- **`create_before_destroy` does not change either claim.** With it set on
  `base`, the old object is destroyed LAST - `middle.destroy`, `base.create`,
  `middle.create`, `base.deposed.destroy` - so a resource's own two steps move
  relative to each other, and that is #43's business. A dependant's destroy
  still precedes its dependency's destroy, and a dependant's create still
  follows its dependency's create.

`testdata/sequence-partial.json` has no root here and is hand-written, which it
says in the file. A no-op dependant of a replaced resource cannot be generated
from `terraform_data`: every one of its attributes is unknown after a
replacement, so anything reading one is always planned as a change. The shape is
realistic on a real provider - a resource reading a subnet's name, where the
name is known and unchanged across the replacement.
