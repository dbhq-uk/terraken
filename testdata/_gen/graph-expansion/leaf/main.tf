variable "seed" {
  type    = string
  default = "x"
}
resource "terraform_data" "inner" {
  triggers_replace = var.seed
  input            = "inner"
}
