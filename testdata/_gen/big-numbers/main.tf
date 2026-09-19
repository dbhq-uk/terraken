variable "n" {
  type    = number
  default = 9007199254740992
}

resource "terraform_data" "probe" {
  input = var.n
}
