// GENERATED - do not edit. assets/_gen/samples.py in this repository.
//
// Real output from the built binary, run with FORCE_COLOR against the fixtures
// in testdata/, with its ANSI converted to spans. The colour is not decoration:
// red means this change can destroy something, and the site carried a
// monochrome capture until 16 Sep 2026, which threw that away.
//
// A .ts module rather than .html files imported with ?raw. Vite resolves ?raw
// happily; tests/content.test.mjs imports this through plain Node, which
// cannot load a .html file at all and fails at module load with
// ERR_UNKNOWN_FILE_EXTENSION before a single assertion runs.
//
// Regenerate whenever the renderer changes:
//   go build -o /tmp/terraken ./cmd/terraken && python3 assets/_gen/samples.py

export const critical = `<span class="t-b">terraken</span>  1 finding  <span class="t-dim">terraform 1.9.8</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">─────────────────────────────────────────</span>  <span class="t-low">1</span>

  <span class="t-b">1 of 1 change were read in full, and the note below is what the rest of this</span>
  <span class="t-b">plan does not say</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate, and that means</span>
  <span class="t-dim">one of two things it does not distinguish: either nothing drifted, or</span>
  <span class="t-dim">refresh never ran and nobody looked</span>

<span class="t-crit">CRITICAL</span> <span class="t-dim">──────────────────────────────────────────────────────────────────</span>  <span class="t-crit">1</span>

  <span class="t-b">azurerm_postgresql_flexible_server.main</span>
  <span class="t-dim">destroy, then create</span>
<span class="t-dim">  ├ </span>holds data, so destroying it loses that data
<span class="t-dim">  ├ </span>an attribute changed that cannot be updated in place
<span class="t-dim">  ├ </span>forces replacement   zone
<span class="t-dim">  └ </span>destroyed before the replacement is created

<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>
<span class="t-crit">1 critical</span>`;

export const heroCritical = `<span class="t-b">terraken</span>  1 finding  <span class="t-dim">terraform 1.9.8</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">───────────────────────────</span>  <span class="t-low">1</span>

  <span class="t-b">1 of 1 change were read in full, and the note below is what</span>
  <span class="t-b">the rest of this plan does not say</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate,</span>
  <span class="t-dim">and that means one of two things it does not distinguish:</span>
  <span class="t-dim">either nothing drifted, or refresh never ran and nobody looked</span>

<span class="t-crit">CRITICAL</span> <span class="t-dim">────────────────────────────────────────────────────</span>  <span class="t-crit">1</span>

  <span class="t-b">azurerm_postgresql_flexible_server.main</span>
  <span class="t-dim">destroy, then create</span>
<span class="t-dim">  ├ </span>holds data, so destroying it loses that data
<span class="t-dim">  ├ </span>an attribute changed that cannot be updated in place
<span class="t-dim">  ├ </span>forces replacement   zone
<span class="t-dim">  └ </span>destroyed before the replacement is created

<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>
<span class="t-crit">1 critical</span>`;

export const missedMove = `<span class="t-b">terraken</span>  2 findings  <span class="t-dim">terraform 1.16.1</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">─────────────────────────────────────────</span>  <span class="t-low">2</span>

  <span class="t-b">1 of 2 changes were read in full, and the 2 notes below are what the rest of</span>
  <span class="t-b">this plan does not say</span>

  <span class="t-dim">1 of 2 changes carry values Terraform will not know until it applies them,</span>
  <span class="t-dim">so no claim about those values can be checked now</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate, and that means</span>
  <span class="t-dim">one of two things it does not distinguish: either nothing drifted, or</span>
  <span class="t-dim">refresh never ran and nobody looked</span>

<span class="t-high">HIGH</span> <span class="t-dim">──────────────────────────────────────────────────────────────────────</span>  <span class="t-high">1</span>

  <span class="t-b">azurerm_subnet.app</span>
  <span class="t-dim">destroy</span>
<span class="t-dim">  └ </span>possible missed moved block
      <span class="t-dim">5 of 5 attributes match azurerm_subnet.application</span>
      <span class="t-dim">moved { from = azurerm_subnet.app  to = azurerm_subnet.application }</span>
      <span class="t-dim">verify the pairing before using that block</span>

<span class="t-dim">INFO</span> <span class="t-dim">──────────────────────────────────────────────────────────────────────</span>  <span class="t-dim">1</span>

  <span class="t-b">azurerm_subnet.application</span>
  <span class="t-dim">create</span>
<span class="t-dim">  ├ </span>these values are not known until apply, so no claim about them can be
<span class="t-dim">  │ </span>checked in review
<span class="t-dim">  │   </span><span class="t-dim">etag</span>
<span class="t-dim">  │   </span><span class="t-dim">id</span>
<span class="t-dim">  └ </span>possible missed moved block
      <span class="t-dim">paired with azurerm_subnet.app, shown above</span>

<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>
<span class="t-high">1 high</span>  <span class="t-dim">1 info</span>`;

export const rewritten = `<span class="t-b">terraken</span>  5 findings  <span class="t-dim">terraform 1.9.8</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">─────────────────────────────────────────</span>  <span class="t-low">1</span>

  <span class="t-b">5 of 5 changes were read in full, and the note below is what the rest of</span>
  <span class="t-b">this plan does not say</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate, and that means</span>
  <span class="t-dim">one of two things it does not distinguish: either nothing drifted, or</span>
  <span class="t-dim">refresh never ran and nobody looked</span>

<span class="t-low">LOW</span> <span class="t-dim">───────────────────────────────────────────────────────────────────────</span>  <span class="t-low">5</span>

  <span class="t-b">aws_ecs_task_definition.api</span>
  <span class="t-dim">update in place</span>
<span class="t-dim">  ├ </span>same text, different whitespace
<span class="t-dim">  │   </span><span class="t-dim">container_definitions</span>
<span class="t-dim">  └ </span><span class="t-b">every attribute this plan shows as changed here is a difference in how the</span>
    <span class="t-b">value is written</span>

  <span class="t-b">aws_iam_policy.pipeline</span>
  <span class="t-dim">update in place</span>
<span class="t-dim">  ├ </span>same JSON, written differently
<span class="t-dim">  │   </span><span class="t-dim">policy</span>
<span class="t-dim">  └ </span><span class="t-b">every attribute this plan shows as changed here is a difference in how the</span>
    <span class="t-b">value is written</span>

  <span class="t-b">aws_instance.bastion</span>
  <span class="t-dim">update in place</span>
<span class="t-dim">  ├ </span>same text, different whitespace
<span class="t-dim">  │   </span><span class="t-dim">user_data</span>
<span class="t-dim">  └ </span>same JSON, written differently
      <span class="t-dim">metadata</span>

  <span class="t-b">aws_lb_target_group.app</span>
  <span class="t-dim">update in place</span>
<span class="t-dim">  ├ </span>null on one side, empty on the other
<span class="t-dim">  │   </span><span class="t-dim">load_balancing_anomaly_mitigation</span>
<span class="t-dim">  │   </span><span class="t-dim">tags</span>
<span class="t-dim">  └ </span><span class="t-b">every attribute this plan shows as changed here is a difference in how the</span>
    <span class="t-b">value is written</span>

  <span class="t-b">aws_security_group.web</span>
  <span class="t-dim">update in place</span>
<span class="t-dim">  ├ </span>same elements, different order
<span class="t-dim">  │   </span><span class="t-dim">ingress[0].cidr_blocks</span>
<span class="t-dim">  ├ </span>same number, written differently
<span class="t-dim">  │   </span><span class="t-dim">ingress[0].from_port</span>
<span class="t-dim">  │   </span><span class="t-dim">ingress[0].to_port</span>
<span class="t-dim">  └ </span><span class="t-b">every attribute this plan shows as changed here is a difference in how the</span>
    <span class="t-b">value is written</span>

<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>
<span class="t-dim">Whitespace is significant in a script, in a YAML document carried as a string</span>
<span class="t-dim">and in anything hashed, so whether a whitespace change matters is yours to</span>
<span class="t-dim">judge.</span>

<span class="t-dim">Each of these classes has a case where the difference is real. Order matters</span>
<span class="t-dim">in a container command, and whitespace matters in a script. This says what</span>
<span class="t-dim">kind of difference each one is and rules on none of them.</span>

<span class="t-dim">A consumer that compares the document byte for byte still sees a change, so</span>
<span class="t-dim">whether a rewritten document matters is yours to judge.</span>

<span class="t-dim">Null and empty are not the same thing to Terraform everywhere, where null can</span>
<span class="t-dim">mean inherit a default and empty means explicitly none, so whether this one</span>
<span class="t-dim">matters is yours to judge.</span>

<span class="t-dim">Order is significant for some attributes, such as a container command or an</span>
<span class="t-dim">ordered rule list, so whether a reordering matters is yours to judge.</span>

<span class="t-dim">A number and its string form are different types, and a type change can</span>
<span class="t-dim">matter, so whether this one does is yours to judge.</span>

<span class="t-low">5 low</span>`;

export const minLevel = `<span class="t-b">terraken</span>  5 findings  <span class="t-dim">terraform 1.16.1</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">─────────────────────────────────────────</span>  <span class="t-low">2</span>

  <span class="t-b">2 of 5 changes were read in full, and the 2 notes below are what the rest of</span>
  <span class="t-b">this plan does not say</span>

  <span class="t-dim">3 of 5 changes carry values Terraform will not know until it applies them,</span>
  <span class="t-dim">so no claim about those values can be checked now</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate, and that means</span>
  <span class="t-dim">one of two things it does not distinguish: either nothing drifted, or</span>
  <span class="t-dim">refresh never ran and nobody looked</span>

<span class="t-crit">CRITICAL</span> <span class="t-dim">──────────────────────────────────────────────────────────────────</span>  <span class="t-crit">1</span>

  <span class="t-b">azurerm_postgresql_flexible_server.main</span>
  <span class="t-dim">destroy, then create</span>
<span class="t-dim">  ├ </span>holds data, so destroying it loses that data
<span class="t-dim">  ├ </span>an attribute changed that cannot be updated in place
<span class="t-dim">  ├ </span>forces replacement   zone
<span class="t-dim">  ├ </span>destroyed before the replacement is created
<span class="t-dim">  ├ </span>these values are not known until apply, so no claim about them can be
<span class="t-dim">  │ </span>checked in review
<span class="t-dim">  │   </span><span class="t-dim">fqdn</span>
<span class="t-dim">  │   </span><span class="t-dim">id</span>
<span class="t-dim">  └ </span>these values are sensitive and are redacted in all output
      <span class="t-dim">administrator_password</span>

<span class="t-high">HIGH</span> <span class="t-dim">──────────────────────────────────────────────────────────────────────</span>  <span class="t-high">1</span>

  <span class="t-b">azurerm_subnet.app</span>
  <span class="t-dim">destroy</span>
<span class="t-dim">  ├ </span>its configuration block was removed
<span class="t-dim">  └ </span>possible missed moved block
      <span class="t-dim">6 of 6 attributes match azurerm_subnet.application</span>
      <span class="t-dim">moved { from = azurerm_subnet.app  to = azurerm_subnet.application }</span>
      <span class="t-dim">verify the pairing before using that block</span>

<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>
<span class="t-crit">1 critical</span>  <span class="t-high">1 high</span>  <span class="t-low">1 low</span>  <span class="t-dim">2 info</span>                       <span class="t-dim">3 below high not shown</span>`;

export const credentials = `<span class="t-b">terraken</span>  5 findings  <span class="t-dim">terraform 1.16.1</span>
<span class="t-dim">━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━</span>

<span class="t-crit">CREDENTIALS IN THE PLAN FILE</span> <span class="t-dim">──────────────────────────────────────────────</span>  <span class="t-crit">6</span>

  <span class="t-b">(root variables)  cloudflare_api_token</span>
  <span class="t-dim">an attribute named as a secret, not marked sensitive</span>

  <span class="t-b">terraform_data.app  input.api_token</span>
  <span class="t-dim">a GitHub token</span>

  <span class="t-b">terraform_data.app  input.database_url</span>
  <span class="t-dim">a connection string with an embedded password</span>

  <span class="t-b">terraform_data.aws  input.access_key_id</span>
  <span class="t-dim">an AWS access key id</span>

  <span class="t-b">terraform_data.cdn  input.edge_handle</span>
  <span class="t-dim">a long, high-entropy string</span>

  <span class="t-b">terraform_data.signing  input.material</span>
  <span class="t-dim">a private key</span>

  Treat this plan file as a secret: store it accordingly, and rotate whatever
  it turns out to hold. Editing the file does not undo the exposure.

<span class="t-low">WHAT THIS PLAN SAYS ABOUT ITSELF</span> <span class="t-dim">──────────────────────────────────────────</span>  <span class="t-low">3</span>

  <span class="t-b">errored   no</span>

  <span class="t-b">complete   yes</span>

  <span class="t-b">applyable   yes</span>

<span class="t-low">HOW MUCH OF THIS COULD BE CHECKED</span> <span class="t-dim">─────────────────────────────────────────</span>  <span class="t-low">3</span>

  <span class="t-b">0 of 5 changes were read in full, and the 3 notes below are what the rest of</span>
  <span class="t-b">this plan does not say</span>

  <span class="t-dim">5 of 5 changes carry values Terraform will not know until it applies them,</span>
  <span class="t-dim">so no claim about those values can be checked now</span>

  <span class="t-dim">this plan records nothing that changed underneath the estate, and that means</span>
  <span class="t-dim">one of two things it does not distinguish: either nothing drifted, or</span>
  <span class="t-dim">refresh never ran and nobody looked</span>

  <span class="t-dim">1 of 1 outputs hold a value Terraform will not know until it applies this,</span>
  <span class="t-dim">so what they will contain cannot be checked now</span>

<span class="t-dim">This is pattern matching over the plan&#x27;s own values: it misses credentials it</span>
<span class="t-dim">does not recognise, and it names values that are not credentials. Treat it as</span>
<span class="t-dim">a reason to check, never as a clean bill of health.</span>

<span class="t-dim">5 info</span>                                              <span class="t-dim">5 below critical not shown</span>`;
