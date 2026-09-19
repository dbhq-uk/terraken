variable "release" {
  type    = string
  default = "v1"
}
resource "terraform_data" "a" {
  count            = 2
  triggers_replace = var.release
  input            = "a"
}
resource "terraform_data" "b" {
  count = 0
  input = terraform_data.a[0].id
}
module "m" {
  source     = "./mod"
  seed       = var.release
  depends_on = [terraform_data.a[0]]
}
