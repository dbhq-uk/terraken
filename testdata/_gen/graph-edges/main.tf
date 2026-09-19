variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  for_each         = toset(["a.b", "other"])
  triggers_replace = var.release
  input            = each.key
}

# A WHOLE instance read, with no attribute after it. Terraform exports the
# indexed form and a bare one, both two segments long.
resource "terraform_data" "whole_read" {
  triggers_replace = terraform_data.base["a.b"].id
  input            = jsonencode(terraform_data.base["a.b"])
}

# depends_on naming a module that exports nothing.
module "empty" {
  source = "./empty"
  seed   = var.release
}
resource "terraform_data" "after_empty" {
  triggers_replace = var.release
  input            = "after_empty"

  depends_on = [module.empty]
}

# An indexed traversal of a module output.
module "obj" {
  source = "./obj"
  seed   = var.release
}
resource "terraform_data" "reads_item" {
  triggers_replace = module.obj.items[0].id
  input            = "reads_item"
}
