variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "a" {
  count            = 2
  triggers_replace = var.release
  input            = "independent"
}

resource "terraform_data" "b" {
  count            = 1
  triggers_replace = terraform_data.a[0].id
  input            = "now depends on a"
}
