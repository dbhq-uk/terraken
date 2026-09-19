variable "release" {
  type    = string
  default = "v1"
}
resource "terraform_data" "base" {
  for_each         = toset(["a.b", "other"])
  triggers_replace = var.release
  input            = each.key
}
# TWO selective traversals of the same instance, in one expression.
resource "terraform_data" "twice" {
  triggers_replace = jsonencode([terraform_data.base["a.b"], terraform_data.base["a.b"].id])
  input            = "twice"
}
