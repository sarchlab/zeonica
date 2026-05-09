# Energy Config Examples

These examples are illustrative only. Values are not process-calibrated silicon measurements.

Inline-only model:

```yaml
energy:
  enabled: true
  units: pJ
  unknown_action_policy: error
  actions:
    pe.inst.ADD: 1.0
    pe.inst.predicate_suppressed: 0.0
    pe.dataflow.send: 0.5
    pe.memory.request_load: 4.0
```

Model-file based configuration:

```yaml
energy:
  enabled: true
  model_file: energy_model.example.yaml
```

Inline action values override the same action from `model_file`; omitted actions and policy remain from the file:

```yaml
energy:
  enabled: true
  model_file: energy_model.example.yaml
  actions:
    pe.inst.ADD: 1.2
```

With `unknown_action_policy: error`, report generation fails when an observed action has no configured energy value. Use `warn` or `zero` to keep generating reports while listing unresolved actions in the energy report.
