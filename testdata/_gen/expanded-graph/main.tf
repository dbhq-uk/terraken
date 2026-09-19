variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = "base ${var.release}"
}

# count: instances are follower[0] and follower[1]; the configuration says
# terraform_data.follower.
resource "terraform_data" "counted" {
  count            = 2
  triggers_replace = terraform_data.base.output
  input            = "counted"
}

# for_each: instances carry string keys.
resource "terraform_data" "keyed" {
  for_each         = toset(["alpha", "beta"])
  triggers_replace = terraform_data.base.output
  input            = each.key
}

# depends_on only: no expression references the base at all.
resource "terraform_data" "ordered" {
  triggers_replace = var.release
  input            = "ordered"

  depends_on = [terraform_data.base]
}

# an indexed module call
module "app" {
  count  = 2
  source = "./mod"
  seed   = terraform_data.base.output
}
