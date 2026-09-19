variable "seed" { type = string }
resource "terraform_data" "kept" {
  triggers_replace = var.seed
  input            = "kept"
}
