variable "release" {
  type    = string
  default = "v1"
}

resource "terraform_data" "base" {
  for_each         = toset(["a.b", "other"])
  triggers_replace = var.release
  input            = each.key
}

# Reads the WHOLE collection and one instance, in one expression. Terraform
# exports the bare reference twice.
resource "terraform_data" "mixed" {
  triggers_replace = jsonencode([values(terraform_data.base)[*].id, terraform_data.base["a.b"].id])
  input            = "mixed"
}

# Reads one instance only.
resource "terraform_data" "selective" {
  triggers_replace = terraform_data.base["a.b"].id
  input            = "selective"
}
