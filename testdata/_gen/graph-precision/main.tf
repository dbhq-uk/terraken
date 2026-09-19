variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = "base ${var.release}"
}

# Selecting ONE instance must not depend on every instance.
resource "terraform_data" "keyed" {
  for_each         = toset(["one", "two"])
  triggers_replace = var.release
  input            = each.key
}
resource "terraform_data" "picks_one" {
  triggers_replace = terraform_data.keyed["one"].output
  input            = "picks_one"
}

# Reading ONE module output must not depend on every output.
module "apple" {
  source = "./leaf"
  seed   = terraform_data.base.output
}
resource "terraform_data" "reads_a" {
  triggers_replace = module.apple.a
  input            = "reads_a"
}

# A module output forwarded through a wrapper.
module "wrapper" {
  source = "./wrap"
  seed   = terraform_data.base.output
}
resource "terraform_data" "reads_wrapped" {
  triggers_replace = module.wrapper.result
  input            = "reads_wrapped"
}

# count = 0 declares a resource and produces none.
resource "terraform_data" "never" {
  count            = 0
  triggers_replace = terraform_data.base.output
  input            = "never"
}
