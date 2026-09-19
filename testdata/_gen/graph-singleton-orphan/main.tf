variable "release" {
  type    = string
  default = "v1"
}
resource "terraform_data" "a" {
  triggers_replace = var.release
  input            = "a"
}
# An empty for_each: the singleton that exists is now an orphan.
resource "terraform_data" "b" {
  for_each = {}
  input    = terraform_data.a.id
}
