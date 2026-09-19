variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  triggers_replace = var.release
  input            = ["a", "b"]
}

# count and for_each are expressions Terraform exports OUTSIDE `expressions`.
resource "terraform_data" "counted" {
  count            = length(terraform_data.base.input)
  triggers_replace = var.release
  input            = "counted"
}
resource "terraform_data" "eached" {
  for_each         = toset(terraform_data.base.input)
  triggers_replace = var.release
  input            = each.key
}

# depends_on on a MODULE CALL: everything inside depends on the base.
module "ordered" {
  source     = "./leaf"
  seed       = var.release
  depends_on = [terraform_data.base]
}
