variable "seed" { type = string }
resource "terraform_data" "hidden" {
  triggers_replace = var.seed
  input            = "hidden"
}
