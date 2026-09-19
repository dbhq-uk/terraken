variable "revision" {
  type    = string
  default = "before"
}

# Its triggers_replace never changes, and both it and input are fully known -
# so after_unknown carries an EMPTY object for each, meaning "no unknowns in
# here" rather than "this changed".
resource "terraform_data" "probe" {
  input            = { revision = var.revision }
  triggers_replace = { stable = "same" }
}

# Nothing set at all: every optional attribute is a known null, and Terraform
# shows only id being created.
resource "terraform_data" "empty" {
}
