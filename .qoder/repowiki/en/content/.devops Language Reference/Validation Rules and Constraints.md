# Validation Rules and Constraints

<cite>
**Referenced Files in This Document**
- [validate.go](file://internal/devlang/validate.go)
- [parser.go](file://internal/devlang/parser.go)
- [lexer.go](file://internal/devlang/lexer.go)
- [ast.go](file://internal/devlang/ast.go)
- [lower.go](file://internal/devlang/lower.go)
- [types.go](file://internal/devlang/types.go)
- [eval.go](file://internal/devlang/eval.go)
- [schema.go](file://internal/plan/schema.go)
- [validate.go](file://internal/plan/validate.go)
- [validate_test.go](file://internal/plan/validate_test.go)
- [compile_test.go](file://internal/devlang/compile_test.go)
- [main.go](file://cmd/devopsctl/main.go)
- [plan.devops](file://plan.devops)
- [plan_resume.devops](file://tests/e2e/plan_resume.devops)
- [comprehensive.devops](file://tests/v0_3/valid/comprehensive.devops)
- [concat.devops](file://tests/v0_3/valid/concat.devops)
- [logical.devops](file://tests/v0_3/valid/logical.devops)
- [ternary.devops](file://tests/v0_3/valid/ternary.devops)
- [type_mismatch.devops](file://tests/v0_3/invalid/type_mismatch.devops)
- [unresolved_var.devops](file://tests/v0_3/invalid/unresolved_var.devops)
- [expr_version.devops](file://tests/v0_3/hash_stability/expr_version.devops)
- [literal_version.devops](file://tests/v0_3/hash_stability/literal_version.devops)
- [step_basic.devops](file://tests/v0_4/valid/step_basic.devops)
- [step_comprehensive.devops](file://tests/v0_4/valid/step_comprehensive.devops)
- [step_multiple_targets.devops](file://tests/v0_4/valid/step_multiple_targets.devops)
- [step_override_inputs.devops](file://tests/v0_4/valid/step_override_inputs.devops)
- [step_duplicate.devops](file://tests/v0_4/invalid/step_duplicate.devops)
- [step_primitive_collision.devops](file://tests/v0_4/invalid/step_primitive_collision.devops)
- [step_undefined.devops](file://tests/v0_4/invalid/step_undefined.devops)
- [step_unknown_primitive.devops](file://tests/v0_4/invalid/step_unknown_primitive.devops)
- [step_with_depends_on.devops](file://tests/v0_4/invalid/step_with_depends_on.devops)
- [step_with_targets.devops](file://tests/v0_4/invalid/step_with_targets.devops)
- [for_basic.devops](file://tests/v0_5/valid/for_basic.devops)
- [for_multiple_loops.devops](file://tests/v0_5/valid/for_multiple_loops.devops)
- [for_with_let_range.devops](file://tests/v0_5/valid/for_with_let_range.devops)
- [for_with_lets.devops](file://tests/v0_5/valid/for_with_lets.devops)
- [nested_step_basic.devops](file://tests/v0_5/valid/nested_step_basic.devops)
- [nested_step_deep.devops](file://tests/v0_5/valid/nested_step_deep.devops)
- [nested_step_override.devops](file://tests/v0_5/valid/nested_step_override.devops)
- [for_non_list_range.devops](file://tests/v0_5/invalid/for_non_list_range.devops)
- [nested_step_cycle_direct.devops](file://tests/v0_5/invalid/nested_step_cycle_direct.devops)
- [nested_step_cycle_indirect.devops](file://tests/v0_5/invalid/nested_step_cycle_indirect.devops)
- [nested_step_self_reference.devops](file://tests/v0_5/invalid/nested_step_self_reference.devops)
- [param_basic.devops](file://tests/v0_6/valid/param_basic.devops)
- [param_required.devops](file://tests/v0_6/valid/param_required.devops)
- [param_duplicate.devops](file://tests/v0_6/invalid/param_duplicate.devops)
- [param_missing_required.devops](file://tests/v0_6/invalid/param_missing_required.devops)
- [param_with_default.devops](file://tests/v0_6/hash_stability/param_with_default.devops)
- [param_manual_expansion.devops](file://tests/v0_6/hash_stability/param_manual_expansion.devops)
</cite>

## Update Summary
**Changes Made**
- Enhanced validation rules documentation to include comprehensive parameter-specific checks for parameter uniqueness, default value validation, parameter shadowing detection, and compile-time determinism enforcement
- Updated language version 0.6 validation coverage to include step parameter validation with detailed error reporting
- Expanded parameter validation documentation with compile-time evaluation constraints and parameter substitution mechanisms
- Added new sections covering parameter-specific validation rules and their integration with step expansion
- Updated troubleshooting guide with parameter validation error scenarios and solutions

## Table of Contents
1. [Introduction](#introduction)
2. [Project Structure](#project-structure)
3. [Core Components](#core-components)
4. [Architecture Overview](#architecture-overview)
5. [Detailed Component Analysis](#detailed-component-analysis)
6. [Comprehensive Validation Tests](#comprehensive-validation-tests)
7. [Dependency Analysis](#dependency-analysis)
8. [Performance Considerations](#performance-considerations)
9. [Troubleshooting Guide](#troubleshooting-guide)
10. [Conclusion](#conclusion)
11. [Appendices](#appendices)

## Introduction
This document explains the validation rules and constraints enforced by the .devops language compiler and planner across multiple language versions, with comprehensive coverage of the new v0.6 language constructs. It covers semantic validation during compilation (type checking, scope resolution, constraint verification), the IR-level validation performed against the execution plan, and how these validations relate to runtime safety guarantees. The documentation now includes extensive coverage of v0.6 enhancements, particularly around parameter validation, parameter uniqueness checks, default value validation, parameter shadowing detection, and compile-time determinism enforcement.

**Updated** Enhanced with comprehensive semantic validation tests covering language versions 0.1, 0.2, 0.3, 0.4, 0.5, and 0.6, including new features like parameter validation, parameter uniqueness checks, default value validation, parameter shadowing detection, and advanced compile-time evaluation.

## Project Structure
The validation pipeline spans several layers and supports multiple language versions with progressively enhanced capabilities:
- Lexical analysis: tokenization of .devops source into tokens.
- Parsing: construction of an AST from tokens.
- Semantic validation: language-version-specific checks on the AST with v0.6 adding parameter validation and step parameter support.
- Lowering: conversion of AST to an intermediate representation (IR) plan with parameter substitution and step expansion.
- IR validation: structural and type checks on the plan.

```mermaid
graph TB
SRC[".devops source file"]
LEX["Lexer<br/>tokenization"]
PARSE["Parser<br/>AST construction"]
SEMVAL01["Semantic Validator v0.1<br/>Legacy validation"]
SEMVAL02["Semantic Validator v0.2<br/>Enhanced validation"]
SEMVAL03["Semantic Validator v0.3<br/>Advanced expression validation"]
SEMVAL04["Semantic Validator v0.4<br/>Step definition validation"]
SEMVAL05["Semantic Validator v0.5<br/>For-loops and nested steps"]
SEMVAL06["Semantic Validator v0.6<br/>Parameter validation and step parameters"]
LOWER01["Lowerer v0.1<br/>AST -> Plan IR (no lets)"]
LOWER02["Lowerer v0.2<br/>AST -> Plan IR (with lets)"]
LOWER03["Lowerer v0.3<br/>AST -> Plan IR (with evaluated lets)"]
LOWER04["Lowerer v0.4<br/>AST -> Plan IR (with step expansion)"]
LOWER05["Lowerer v0.5<br/>AST -> Plan IR (with step expansion + for-loop unrolling)"]
LOWER06["Lowerer v0.6<br/>AST -> Plan IR (with parameter substitution)"]
IRVAL["IR Validator<br/>Plan checks"]
OUT["Execution Plan JSON"]
SRC --> LEX --> PARSE --> SEMVAL01 --> LOWER01 --> IRVAL --> OUT
PARSE --> SEMVAL02 --> LOWER02 --> IRVAL --> OUT
PARSE --> SEMVAL03 --> LOWER03 --> IRVAL --> OUT
PARSE --> SEMVAL04 --> LOWER04 --> IRVAL --> OUT
PARSE --> SEMVAL05 --> LOWER05 --> IRVAL --> OUT
PARSE --> SEMVAL06 --> LOWER06 --> IRVAL --> OUT
```

**Diagram sources**
- [lexer.go](file://internal/devlang/lexer.go#L60-L100)
- [parser.go](file://internal/devlang/parser.go#L28-L78)
- [validate.go](file://internal/devlang/validate.go#L23-L194)
- [validate.go](file://internal/devlang/validate.go#L196-L315)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [lower.go](file://internal/devlang/lower.go#L10-L65)
- [lower.go](file://internal/devlang/lower.go#L92-L148)
- [lower.go](file://internal/devlang/lower.go#L180-L282)
- [lower.go](file://internal/devlang/lower.go#L284-L598)
- [lower.go](file://internal/devlang/lower.go#L600-L707)
- [validate.go](file://internal/plan/validate.go#L7-L94)

**Section sources**
- [lexer.go](file://internal/devlang/lexer.go#L1-L247)
- [parser.go](file://internal/devlang/parser.go#L1-L495)
- [validate.go](file://internal/devlang/validate.go#L1-L2011)
- [lower.go](file://internal/devlang/lower.go#L1-L870)
- [validate.go](file://internal/plan/validate.go#L1-L95)

## Core Components
- Semantic validator for language version 0.1: rejects unsupported constructs, enforces duplicate detection, validates node-level constraints, and performs primitive-specific input checks.
- Semantic validator for language version 0.2: extends v0.1 validation with let binding support, literal type restrictions, and enhanced duplicate detection.
- Semantic validator for language version 0.3: extends v0.2 validation with advanced expression evaluation, type checking, constant folding, and comprehensive error reporting.
- Semantic validator for language version 0.4: extends v0.3 validation with step definition support, duplicate step detection, primitive type collision prevention, and step expansion rules.
- Semantic validator for language version 0.5: extends v0.4 validation with for-loop support, nested step validation, circular dependency detection, and compile-time evaluation constraints.
- Semantic validator for language version 0.6: extends v0.5 validation with comprehensive parameter validation, parameter uniqueness checks, default value validation, parameter shadowing detection, and compile-time determinism enforcement.
- IR validator: ensures plan-level correctness (presence of required fields, known targets/nodes, valid failure policies, and primitive inputs).
- Lowerer: transforms AST into a plan with concrete values, enforcing that only supported expressions are lowered and steps are expanded to concrete nodes with parameter substitution.

Key responsibilities:
- Language version 0.1: Reject unsupported constructs (let, for, step, module) and enforce strict validation rules.
- Language version 0.2: Support let bindings with literal type restrictions, enhanced duplicate detection, and improved error reporting.
- Language version 0.3: Support advanced expressions with type checking, constant folding, comprehensive error reporting, and enhanced validation.
- Language version 0.4: Support step definitions with input merging, duplicate detection, primitive collision prevention, and step expansion lowering.
- Language version 0.5: Support for-loops with compile-time evaluation, nested step validation with circular dependency detection, and advanced step expansion.
- Language version 0.6: Support step parameters with parameter uniqueness validation, default value type checking and evaluation, parameter shadowing detection, and compile-time determinism enforcement.
- Scope resolution via symbol tables for targets, nodes, steps, for-loops, and parameters.
- Constraint checks for node types, targets, depends_on, failure_policy, primitive inputs, step definitions, for-loop ranges, and parameter validation.
- IR-level checks mirroring AST-level checks to catch issues early.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L23-L194)
- [validate.go](file://internal/devlang/validate.go#L196-L315)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [validate.go](file://internal/plan/validate.go#L7-L94)
- [lower.go](file://internal/devlang/lower.go#L10-L870)

## Architecture Overview
The validation architecture separates concerns across stages and supports multiple language versions with progressive enhancement:
- Language-level checks occur before lowering to ensure only supported constructs are accepted.
- IR-level checks ensure the plan is structurally sound and consistent with runtime expectations.
- Language version 0.6 introduces comprehensive parameter validation with compile-time evaluation and parameter substitution.
- Step expansion lowers step references to concrete node definitions with proper parameter resolution and input merging.
- For-loop unrolling expands loop bodies into concrete node definitions at compile time.
- Parameter validation ensures compile-time determinism and prevents runtime parameter resolution issues.

```mermaid
sequenceDiagram
participant CLI as "CLI"
participant DL as "devlang.CompileFileV0_1/V0_2/V0_3/V0_4/V0_5/V0_6"
participant P as "Parser"
participant SV as "Semantic Validator"
participant TE as "Type Checker"
participant EE as "Expression Evaluator"
participant L as "Lowerer"
participant PV as "Plan Validator"
CLI->>DL : compile .devops (v0.1, v0.2, v0.3, v0.4, v0.5, or v0.6)
DL->>P : ParseFile()
P-->>DL : AST or parse errors
alt v0.1
DL->>SV : ValidateV0_1(AST)
else v0.2
DL->>SV : ValidateV0_2(AST) with LetEnv
else v0.3
DL->>SV : ValidateV0_3(AST) with LetEnv
SV->>TE : typeCheckExpr() for each let
TE-->>SV : typed expressions
SV->>EE : evaluateExpr() for each let
EE-->>SV : evaluated literals
else v0.4
DL->>SV : ValidateV0_4(AST) with LetEnv + Steps
SV->>TE : typeCheckExpr() for each let
TE-->>SV : typed expressions
SV->>EE : evaluateExpr() for each let
EE-->>SV : evaluated literals
SV->>SV : Validate step definitions
SV->>SV : Collect steps map
else v0.5
DL->>SV : ValidateV0_5(AST) with LetEnv + Steps + ForLoops
SV->>TE : typeCheckExpr() for each let
TE-->>SV : typed expressions
SV->>EE : evaluateExpr() for each let
EE-->>SV : evaluated literals
SV->>SV : Validate for-loops
SV->>SV : Validate nested steps
SV->>SV : Detect step cycles
SV->>SV : Collect steps map + for-loops
else v0.6
DL->>SV : ValidateV0_6(AST) with LetEnv + Steps + ForLoops + Params
SV->>TE : typeCheckExpr() for each let
TE-->>SV : typed expressions
SV->>EE : evaluateExpr() for each let
EE-->>SV : evaluated literals
SV->>SV : Validate parameters (uniqueness, defaults, shadowing)
SV->>SV : Validate step definitions
SV->>SV : Detect step cycles
SV->>SV : Collect steps map + for-loops + params
end
SV-->>DL : semantic errors or OK + LetEnv + Steps (+ ForLoops + Params)
alt errors present
DL-->>CLI : return errors
else no errors
DL->>L : LowerToPlan/LowerToPlanV0_2/LowerToPlanV0_3/LowerToPlanV0_4/LowerToPlanV0_5/LowerToPlanV0_6(AST, LetEnv, Steps, ForLoops, Params)
L->>L : Expand steps with parameter substitution
L->>L : Unroll for-loops to concrete nodes
L-->>DL : Plan IR
DL->>PV : plan.Validate(Plan)
PV-->>DL : plan errors or OK
alt errors present
DL-->>CLI : return errors
else no errors
DL-->>CLI : return Plan + JSON
end
end
```

**Diagram sources**
- [main.go](file://cmd/devopsctl/main.go#L49-L72)
- [validate.go](file://internal/devlang/validate.go#L455-L491)
- [parser.go](file://internal/devlang/parser.go#L28-L39)
- [validate.go](file://internal/devlang/validate.go#L23-L194)
- [validate.go](file://internal/devlang/validate.go#L196-L315)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)
- [lower.go](file://internal/devlang/lower.go#L10-L179)
- [lower.go](file://internal/devlang/lower.go#L180-L282)
- [lower.go](file://internal/devlang/lower.go#L284-L598)
- [lower.go](file://internal/devlang/lower.go#L600-L707)
- [lower.go](file://internal/devlang/lower.go#L709-L870)
- [validate.go](file://internal/plan/validate.go#L7-L94)

## Detailed Component Analysis

### Semantic Validation (Language Version 0.1)
Semantic validation enforces:
- Unsupported constructs are rejected outright.
- Duplicate target and node declarations are detected.
- Targets referenced by nodes must exist.
- depends_on entries must reference existing nodes.
- Node type must be a known primitive.
- Failure policy must be one of the allowed values.
- Primitive-specific input constraints are checked.

```mermaid
flowchart TD
Start(["ValidateV0_1"]) --> RejectUnsupported["Reject unsupported constructs"]
RejectUnsupported --> DupTargets["Check duplicate targets"]
DupTargets --> DupNodes["Check duplicate nodes"]
DupNodes --> TargetsExist["Verify targets exist"]
TargetsExist --> DepsExist["Verify depends_on nodes exist"]
DepsExist --> TypeKnown["Check primitive type"]
TypeKnown --> FPValid["Check failure_policy"]
FPValid --> PrimInputs["Validate primitive inputs"]
PrimInputs --> End(["Return errors"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L196-L315)

Key rules and diagnostics:
- Unsupported constructs: let, for, step, module are rejected with explicit messages indicating language version 0.1 limitations.
- Duplicate declarations: duplicate target or node names produce errors with precise positions.
- Unknown references: unknown target or node in depends_on produces errors with positions.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L200-L227)
- [validate.go](file://internal/devlang/validate.go#L234-L261)
- [validate.go](file://internal/devlang/validate.go#L264-L312)
- [validate.go](file://internal/devlang/validate.go#L317-L382)

### Semantic Validation (Language Version 0.2)
Semantic validation enforces enhanced rules for language version 0.2:
- Unsupported constructs are rejected (for, step, module) while allowing let bindings.
- Let bindings are collected and validated for literal type restrictions.
- Duplicate let, target, and node declarations are detected.
- Targets referenced by nodes must exist and cannot reference let bindings.
- depends_on entries must reference existing nodes.
- Node type must be a known primitive.
- Failure policy must be one of the allowed values.
- Primitive-specific input constraints are checked with let resolution.

```mermaid
flowchart TD
Start(["ValidateV0_2"]) --> RejectUnsupported["Reject unsupported constructs (for, step, module)"]
RejectUnsupported --> CollectLets["Collect let bindings"]
CollectLets --> LetTypes["Validate literal types"]
LetTypes --> DupLets["Check duplicate let bindings"]
DupLets --> DupTargets["Check duplicate targets"]
DupTargets --> DupNodes["Check duplicate nodes"]
DupNodes --> TargetsExist["Verify targets exist and not let bindings"]
TargetsExist --> DepsExist["Verify depends_on nodes exist"]
DepsExist --> TypeKnown["Check primitive type"]
TypeKnown --> FPValid["Check failure_policy"]
FPValid --> PrimInputs["Validate primitive inputs with let resolution"]
PrimInputs --> End(["Return errors + LetEnv"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L23-L194)

Key rules and diagnostics for v0.2:
- Unsupported constructs: for, step, module are rejected with explicit messages indicating language version 0.2 limitations.
- Let bindings: supported with literal type restrictions (string, bool, or list of string literals).
- Duplicate declarations: duplicate let, target, or node names produce errors with precise positions.
- Let binding restrictions: let bindings cannot be used in targets; targets must reference target declarations.
- Unknown references: unknown target or node in depends_on produces errors with positions.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L28-L50)
- [validate.go](file://internal/devlang/validate.go#L56-L92)
- [validate.go](file://internal/devlang/validate.go#L98-L125)
- [validate.go](file://internal/devlang/validate.go#L127-L191)
- [validate.go](file://internal/devlang/validate.go#L317-L382)

### Semantic Validation (Language Version 0.3)
Semantic validation enforces the most comprehensive rules for language version 0.3:
- Unsupported constructs are rejected (for, step, module) while allowing advanced let bindings with expressions.
- Let bindings are collected with full expression support and undergo type checking.
- Expression evaluation performs constant folding to convert expressions to literals.
- Duplicate let, target, and node declarations are detected.
- Targets referenced by nodes must exist and cannot reference let bindings.
- depends_on entries must reference existing nodes.
- Node type must be a known primitive.
- Failure policy must be one of the allowed values.
- Primitive-specific input constraints are checked with fully evaluated let resolution.

```mermaid
flowchart TD
Start(["ValidateV0_3"]) --> RejectUnsupported["Reject unsupported constructs (for, step, module)"]
RejectUnsupported --> CollectLets["Collect let bindings with expressions"]
CollectLets --> TypeCheck["Type check all let expressions"]
TypeCheck --> EvalExpr["Evaluate expressions to literals"]
EvalExpr --> DupLets["Check duplicate let bindings"]
DupLets --> DupTargets["Check duplicate targets"]
DupTargets --> DupNodes["Check duplicate nodes"]
DupNodes --> TargetsExist["Verify targets exist and not let bindings"]
TargetsExist --> DepsExist["Verify depends_on nodes exist"]
DepsExist --> TypeKnown["Check primitive type"]
TypeKnown --> FPValid["Check failure_policy"]
FPValid --> PrimInputs["Validate primitive inputs with evaluated let resolution"]
PrimInputs --> End(["Return errors + Evaluated LetEnv"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

Key rules and diagnostics for v0.3:
- Unsupported constructs: for, step, module are rejected with explicit messages indicating language version 0.3 limitations.
- Advanced let bindings: support expressions including binary operations, logical operators, equality comparisons, and ternary expressions.
- Type checking: comprehensive type inference with three distinct types (string, bool, string[]) and strict type enforcement.
- Expression evaluation: constant folding converts expressions to concrete literals at compile time.
- Duplicate declarations: duplicate let, target, or node names produce errors with precise positions.
- Let binding restrictions: let bindings cannot be used in targets; targets must reference target declarations.
- Unknown references: unresolved identifiers in expressions produce detailed error messages.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L498-L520)
- [validate.go](file://internal/devlang/validate.go#L526-L543)
- [validate.go](file://internal/devlang/validate.go#L549-L557)
- [validate.go](file://internal/devlang/validate.go#L563-L572)
- [validate.go](file://internal/devlang/validate.go#L581-L608)
- [validate.go](file://internal/devlang/validate.go#L611-L674)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

### Semantic Validation (Language Version 0.4)
Semantic validation enforces the most comprehensive rules for language version 0.4:
- Unsupported constructs are rejected (for, module) while allowing advanced let bindings and step definitions.
- Let bindings are collected with full expression support and undergo type checking.
- Expression evaluation performs constant folding to convert expressions to literals.
- Step definitions are collected and validated for duplicates and primitive collisions.
- Step validation enforces that steps cannot specify targets or depends_on and must have a valid primitive type.
- Duplicate step detection prevents naming conflicts with primitive types and other steps.
- Node validation supports both primitive types and step references with proper input merging.
- Input merging allows steps to define defaults with node-level overrides.
- Failure policy inheritance allows steps to define defaults with node-level overrides.

```mermaid
flowchart TD
Start(["ValidateV0_4"]) --> RejectUnsupported["Reject unsupported constructs (for, module)"]
RejectUnsupported --> CollectLets["Collect let bindings with expressions"]
CollectLets --> TypeCheck["Type check all let expressions"]
TypeCheck --> EvalExpr["Evaluate expressions to literals"]
EvalExpr --> DupLets["Check duplicate let bindings"]
DupLets --> CollectSteps["Collect step definitions"]
CollectSteps --> ValidateSteps["Validate step definitions"]
ValidateSteps --> DupSteps["Check duplicate step names"]
DupSteps --> PrimitiveCollision["Check primitive type collisions"]
PrimitiveCollision --> BuildSymbolTables["Build targets, nodes, and steps tables"]
BuildSymbolTables --> ValidateNodes["Validate nodes with step expansion"]
ValidateNodes --> MergeInputs["Merge step inputs with node overrides"]
MergeInputs --> End(["Return errors + LetEnv + Steps"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L717-L1011)

Key rules and diagnostics for v0.4:
- Unsupported constructs: for, module are rejected with explicit messages indicating language version 0.4 limitations.
- Advanced let bindings: support expressions including binary operations, logical operators, equality comparisons, and ternary expressions.
- Type checking: comprehensive type inference with three distinct types (string, bool, string[]) and strict type enforcement.
- Expression evaluation: constant folding converts expressions to concrete literals at compile time.
- Step definition validation:
  - Steps cannot specify targets or depends_on (these belong to node instantiations)
  - Steps must have a valid primitive type (not another step)
  - Step names cannot collide with primitive types
  - Duplicate step names are rejected
- Input merging and override semantics:
  - Steps define default inputs that nodes can override
  - Node-level inputs take precedence over step defaults
  - Failure policy can be inherited from step or overridden by node
- Duplicate detection: duplicate let, target, node, or step names produce errors with precise positions.
- Let binding restrictions: let bindings cannot be used in targets; targets must reference target declarations.
- Unknown references: unresolved identifiers in expressions or unknown step types produce detailed error messages.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L717-L744)
- [validate.go](file://internal/devlang/validate.go#L751-L781)
- [validate.go](file://internal/devlang/validate.go#L787-L802)
- [validate.go](file://internal/devlang/validate.go#L804-L894)
- [validate.go](file://internal/devlang/validate.go#L900-L1008)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

### Semantic Validation (Language Version 0.5)
Semantic validation enforces the most comprehensive rules for language version 0.5:
- Unsupported constructs are rejected (module) while allowing advanced let bindings, step definitions, and for-loops.
- Let bindings are collected with full expression support and undergo type checking.
- Expression evaluation performs constant folding to convert expressions to literals.
- Step definitions are collected and validated for duplicates, primitive collisions, and nested step dependencies.
- Nested step validation enforces that steps can reference other steps with circular dependency detection.
- For-loop validation enforces compile-time evaluation constraints and body restrictions.
- Step expansion supports recursive expansion with memoization.
- For-loop unrolling expands loop bodies into concrete node definitions.

```mermaid
flowchart TD
Start(["ValidateV0_5"]) --> RejectUnsupported["Reject unsupported constructs (module)"]
RejectUnsupported --> CollectLets["Collect let bindings with expressions"]
CollectLets --> TypeCheck["Type check all let expressions"]
TypeCheck --> EvalExpr["Evaluate expressions to literals"]
EvalExpr --> DupLets["Check duplicate let bindings"]
DupLets --> CollectSteps["Collect step definitions"]
CollectSteps --> ValidateSteps["Validate step definitions"]
ValidateSteps --> DupSteps["Check duplicate step names"]
DupSteps --> PrimitiveCollision["Check primitive type collisions"]
PrimitiveCollision --> BuildStepGraph["Build step dependency graph"]
BuildStepGraph --> DetectCycles["Detect step cycles (DFS)"]
DetectCycles --> ValidateStepTypes["Validate step type resolution"]
ValidateStepTypes --> CollectForLoops["Collect for-loops"]
CollectForLoops --> ValidateForLoops["Validate for-loop ranges and bodies"]
ValidateForLoops --> BuildSymbolTables["Build targets, nodes, steps, and for-loops tables"]
BuildSymbolTables --> ValidateNodes["Validate nodes with step expansion"]
ValidateNodes --> MergeInputs["Merge step inputs with node overrides"]
MergeInputs --> End(["Return errors + LetEnv + Steps + ForLoops"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1413-L1520)

Key rules and diagnostics for v0.5:
- Unsupported constructs: module is rejected with explicit messages indicating language version 0.5 limitations.
- Advanced let bindings: support expressions including binary operations, logical operators, equality comparisons, and ternary expressions.
- Type checking: comprehensive type inference with three distinct types (string, bool, string[]) and strict type enforcement.
- Expression evaluation: constant folding converts expressions to concrete literals at compile time.
- Step definition validation:
  - Steps cannot specify targets or depends_on (these belong to node instantiations)
  - Steps must have a valid type (primitive or another step)
  - Step names cannot collide with primitive types
  - Duplicate step names are rejected
  - Step dependency graph is built for cycle detection
  - Step types must eventually resolve to primitives
- Nested step validation:
  - Steps can reference other steps (v0.5 feature)
  - Circular dependencies are detected using DFS with cycle path reporting
  - Step type resolution ensures eventual primitive types
- For-loop validation:
  - For-loops are allowed with compile-time evaluation constraints
  - Range must evaluate to a literal list of strings
  - For-loop bodies may only contain node declarations
  - Nested for-loops are not allowed
- Step expansion and for-loop unrolling:
  - Steps are recursively expanded to primitive forms
  - For-loops are unrolled into concrete node definitions
  - Loop variables are substituted in node names and string inputs
- Duplicate detection: duplicate let, target, node, or step names produce errors with precise positions.
- Let binding restrictions: let bindings cannot be used in targets; targets must reference target declarations.
- Unknown references: unresolved identifiers in expressions or unknown step types produce detailed error messages.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L1052-L1075)
- [validate.go](file://internal/devlang/validate.go#L1081-L1132)
- [validate.go](file://internal/devlang/validate.go#L1134-L1222)
- [validate.go](file://internal/devlang/validate.go#L1198-L1222)
- [validate.go](file://internal/devlang/validate.go#L1233-L1294)
- [validate.go](file://internal/devlang/validate.go#L1413-L1520)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

### Semantic Validation (Language Version 0.6)
Semantic validation enforces the most comprehensive rules for language version 0.6:
- Unsupported constructs are rejected (module) while allowing advanced let bindings, step definitions, for-loops, and step parameters.
- Let bindings are collected with full expression support and undergo type checking.
- Expression evaluation performs constant folding to convert expressions to literals.
- Step definitions are collected and validated for duplicates, primitive collisions, nested step dependencies, and parameter validation.
- Parameter validation enforces parameter uniqueness, default value type checking and evaluation, parameter shadowing detection, and compile-time determinism.
- Nested step validation enforces that steps can reference other steps with circular dependency detection.
- For-loop validation enforces compile-time evaluation constraints and body restrictions.
- Step expansion supports recursive expansion with memoization and parameter substitution.
- For-loop unrolling expands loop bodies into concrete node definitions with parameter substitution.

```mermaid
flowchart TD
Start(["ValidateV0_6"]) --> RejectUnsupported["Reject unsupported constructs (module)"]
RejectUnsupported --> CollectLets["Collect let bindings with expressions"]
CollectLets --> TypeCheck["Type check all let expressions"]
TypeCheck --> EvalExpr["Evaluate expressions to literals"]
EvalExpr --> DupLets["Check duplicate let bindings"]
DupLets --> CollectSteps["Collect step definitions"]
CollectSteps --> ValidateSteps["Validate step definitions"]
ValidateSteps --> ValidateParams["Validate step parameters"]
ValidateParams --> ParamUniqueness["Check parameter name uniqueness"]
ParamUniqueness --> ParamDefaults["Type-check and evaluate parameter defaults"]
ParamDefaults --> ParamShadowing["Detect parameter shadowing"]
ParamShadowing --> DupSteps["Check duplicate step names"]
DupSteps --> PrimitiveCollision["Check primitive type collisions"]
PrimitiveCollision --> BuildStepGraph["Build step dependency graph"]
BuildStepGraph --> DetectCycles["Detect step cycles (DFS)"]
DetectCycles --> ValidateStepTypes["Validate step type resolution"]
ValidateStepTypes --> CollectForLoops["Collect for-loops"]
CollectForLoops --> ValidateForLoops["Validate for-loop ranges and bodies"]
ValidateForLoops --> BuildSymbolTables["Build targets, nodes, steps, for-loops, and params tables"]
BuildSymbolTables --> ValidateNodes["Validate nodes with step expansion and parameter validation"]
ValidateNodes --> MergeInputs["Merge step inputs with node overrides"]
MergeInputs --> End(["Return errors + LetEnv + Steps + ForLoops + Params"])
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [validate.go](file://internal/devlang/validate.go#L1669-L1711)
- [validate.go](file://internal/devlang/validate.go#L1750-L1769)
- [validate.go](file://internal/devlang/validate.go#L1775-L1850)
- [validate.go](file://internal/devlang/validate.go#L1881-L1971)

Key rules and diagnostics for v0.6:
- Unsupported constructs: module is rejected with explicit messages indicating language version 0.6 limitations.
- Advanced let bindings: support expressions including binary operations, logical operators, equality comparisons, and ternary expressions.
- Type checking: comprehensive type inference with three distinct types (string, bool, string[]) and strict type enforcement.
- Expression evaluation: constant folding converts expressions to concrete literals at compile time.
- Step definition validation:
  - Steps cannot specify targets or depends_on (these belong to node instantiations)
  - Steps must have a valid type (primitive or another step)
  - Step names cannot collide with primitive types
  - Duplicate step names are rejected
  - Step dependency graph is built for cycle detection
  - Step types must eventually resolve to primitives
- Parameter validation (v0.6 new feature):
  - Parameters must have unique names within a step definition
  - Parameter default values are type-checked independently of let bindings
  - Parameter defaults are evaluated once per step definition (compile-time determinism)
  - Parameter defaults are constant-folded to concrete literals
  - Parameter shadowing is detected and reported with detailed error messages
  - Parameter names cannot shadow step names or primitive types
- Nested step validation:
  - Steps can reference other steps (v0.5 feature)
  - Circular dependencies are detected using DFS with cycle path reporting
  - Step type resolution ensures eventual primitive types
- For-loop validation:
  - For-loops are allowed with compile-time evaluation constraints
  - Range must evaluate to a literal list of strings
  - For-loop bodies may only contain node declarations
  - Nested for-loops are not allowed
- Step expansion and for-loop unrolling:
  - Steps are recursively expanded to primitive forms with parameter substitution
  - For-loops are unrolled into concrete node definitions
  - Loop variables are substituted in node names and string inputs
  - Parameter values are substituted in step inputs during expansion
- Duplicate detection: duplicate let, target, node, step, or parameter names produce errors with precise positions.
- Let binding restrictions: let bindings cannot be used in targets; targets must reference target declarations.
- Unknown references: unresolved identifiers in expressions, unknown step types, or undefined parameters produce detailed error messages.
- Primitive types: only allowed primitives are accepted; others produce errors.
- Failure policy: must be one of halt, continue, rollback; otherwise error.
- Primitive inputs:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L1560-L1582)
- [validate.go](file://internal/devlang/validate.go#L1669-L1711)
- [validate.go](file://internal/devlang/validate.go#L1750-L1769)
- [validate.go](file://internal/devlang/validate.go#L1775-L1850)
- [validate.go](file://internal/devlang/validate.go#L1881-L1971)
- [ast.go](file://internal/devlang/ast.go#L65-L81)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

### Type System and Expression Evaluation
The v0.3 type system introduces a comprehensive type checking framework:

**Type System:**
- TypeString: Represents string literals and string expressions
- TypeBool: Represents boolean literals and boolean expressions  
- TypeStringList: Represents lists of strings with special handling

**Expression Evaluation:**
- Binary expressions: + (string concatenation), && (logical AND), || (logical OR), == (equality), != (inequality)
- Ternary expressions: condition ? true_value : false_value with type constraint enforcement
- Constant folding: expressions are evaluated at compile time to produce concrete literals
- Recursive evaluation: nested expressions are resolved through recursive evaluation

```mermaid
flowchart TD
TypeSystem["Type System"] --> String["TypeString<br/>string literals"]
TypeSystem --> Bool["TypeBool<br/>boolean literals"]
TypeSystem --> StringList["TypeStringList<br/>lists of strings"]
ExprEval["Expression Evaluation"] --> Binary["Binary Operations<br/>+, &&, ||, ==, !="]
ExprEval --> Ternary["Ternary Expressions<br/>condition ? true : false"]
ExprEval --> ConstFold["Constant Folding<br/>compile-time evaluation"]
Binary --> Add["String Concatenation<br/>string + string → string"]
Binary --> And["Logical AND<br/>bool && bool → bool"]
Binary --> Or["Logical OR<br/>bool || bool → bool"]
Binary --> Eq["Equality<br/>T == T → bool"]
Binary --> Neq["Inequality<br/>T != T → bool"]
Ternary --> CondCheck["Condition Type<br/>must be bool"]
Ternary --> BranchCheck["Branch Types<br/>must match"]
```

**Diagram sources**
- [types.go](file://internal/devlang/types.go#L5-L25)
- [types.go](file://internal/devlang/types.go#L76-L142)
- [types.go](file://internal/devlang/types.go#L144-L174)
- [eval.go](file://internal/devlang/eval.go#L49-L149)
- [eval.go](file://internal/devlang/eval.go#L151-L172)

**Section sources**
- [types.go](file://internal/devlang/types.go#L5-L25)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

### IR-Level Validation
After lowering, the plan is validated for structural correctness:
- Plan-level fields: version, targets, nodes must be present and non-empty.
- Target-level fields: id and address must be present.
- Node-level fields: id, type, targets must be present; targets must reference known targets; depends_on must reference known nodes; failure_policy must be one of allowed values.
- Primitive-specific checks mirror AST-level checks.

```mermaid
flowchart TD
IRStart(["Plan.Validate"]) --> PlanFields["Check plan fields"]
PlanFields --> TargetsIdx["Index targets and nodes"]
TargetsIdx --> TargetChecks["Validate targets"]
TargetChecks --> NodeChecks["Validate nodes"]
NodeChecks --> PrimChecks["Validate primitives"]
PrimChecks --> IREnd(["Return errors"])
```

**Diagram sources**
- [validate.go](file://internal/plan/validate.go#L7-L94)
- [schema.go](file://internal/plan/schema.go#L12-L39)

**Section sources**
- [validate.go](file://internal/plan/validate.go#L7-L94)
- [schema.go](file://internal/plan/schema.go#L12-L39)

### Lowering and Expression Evaluation
Lowering converts AST expressions into concrete values for the plan with different approaches for each language version:

**Language Version 0.1 Lowering:**
- String literals and boolean literals are preserved.
- Lists are lowered into arrays of strings when all elements are string literals.
- Identifiers are not lowered as values in v0.1; encountering an identifier as a value produces an internal error.
- Unsupported expression nodes produce internal errors.

**Language Version 0.2 Lowering:**
- String literals and boolean literals are preserved.
- Lists are lowered into arrays of strings when all elements are string literals.
- Identifiers are resolved using the let environment; if not found, produces an internal error.
- Unsupported expression nodes produce internal errors.

**Language Version 0.3 Lowering:**
- String literals, boolean literals, and list literals are preserved.
- All let expressions are fully evaluated to concrete literals through constant folding.
- Identifiers are resolved using the evaluated let environment.
- Complex expressions are evaluated to their final literal values.

**Language Version 0.4 Lowering:**
- String literals, boolean literals, and list literals are preserved.
- All let expressions are fully evaluated to concrete literals through constant folding.
- Identifiers are resolved using the evaluated let environment.
- Complex expressions are evaluated to their final literal values.
- Step expansion transforms step references into concrete node definitions with merged inputs and proper failure policy inheritance.

**Language Version 0.5 Lowering:**
- String literals, boolean literals, and list literals are preserved.
- All let expressions are fully evaluated to concrete literals through constant folding.
- Identifiers are resolved using the evaluated let environment.
- Complex expressions are evaluated to their final literal values.
- Step expansion recursively transforms nested step references into concrete node definitions with merged inputs.
- For-loop unrolling expands loop bodies into concrete node definitions with variable substitution.
- Loop variables are substituted in node names and string inputs.

**Language Version 0.6 Lowering:**
- String literals, boolean literals, and list literals are preserved.
- All let expressions are fully evaluated to concrete literals through constant folding.
- Identifiers are resolved using the evaluated let environment.
- Complex expressions are evaluated to their final literal values.
- Step expansion recursively transforms nested step references into concrete node definitions with merged inputs.
- Parameter substitution replaces parameter references with provided values or defaults.
- For-loop unrolling expands loop bodies into concrete node definitions with variable substitution and parameter substitution.
- Loop variables and parameters are substituted in node names, string inputs, and expression values.

```mermaid
flowchart TD
LStart(["LowerToPlan/LowerToPlanV0_2/LowerToPlanV0_3/LowerToPlanV0_4/LowerToPlanV0_5/LowerToPlanV0_6"]) --> Collect["Collect targets and nodes"]
Collect --> ForEachNode["For each NodeDecl"]
ForEachNode --> CheckStepRef{"References Step?"}
CheckStepRef --> |Yes| ExpandStep["Expand step to concrete node"]
ExpandStep --> BuildParamEnv["Build parameter environment"]
BuildParamEnv --> ParamSubstitution["Substitute parameters in step inputs"]
ParamSubstitution --> MergeInputs["Merge step defaults + node overrides"]
MergeInputs --> LowerInputs["Lower inputs with let resolution"]
CheckStepRef --> |No| LowerInputs
LowerInputs --> CheckForLoop{"For-loop?"}
CheckForLoop --> |Yes| UnrollLoop["Unroll for-loop with variable substitution"]
UnrollLoop --> ParamSubstitution2["Apply parameter substitution"]
ParamSubstitution2 --> AppendNode["Append expanded nodes to plan"]
CheckForLoop --> |No| AppendNode
AppendNode --> LEnd(["Done"])
```

**Diagram sources**
- [lower.go](file://internal/devlang/lower.go#L10-L91)
- [lower.go](file://internal/devlang/lower.go#L92-L179)
- [lower.go](file://internal/devlang/lower.go#L180-L282)
- [lower.go](file://internal/devlang/lower.go#L284-L598)
- [lower.go](file://internal/devlang/lower.go#L600-L707)
- [lower.go](file://internal/devlang/lower.go#L709-L870)
- [validate.go](file://internal/devlang/validate.go#L563-L572)
- [validate.go](file://internal/devlang/validate.go#L1025)
- [validate.go](file://internal/devlang/validate.go#L1534)

**Section sources**
- [lower.go](file://internal/devlang/lower.go#L10-L91)
- [lower.go](file://internal/devlang/lower.go#L92-L179)
- [lower.go](file://internal/devlang/lower.go#L180-L282)
- [lower.go](file://internal/devlang/lower.go#L284-L598)
- [lower.go](file://internal/devlang/lower.go#L600-L707)
- [lower.go](file://internal/devlang/lower.go#L709-L870)
- [validate.go](file://internal/devlang/validate.go#L563-L572)
- [validate.go](file://internal/devlang/validate.go#L1025)
- [validate.go](file://internal/devlang/validate.go#L1534)

### Step Definition Validation and Expansion
Language version 0.4 introduces comprehensive step definition validation and expansion rules:

**Step Definition Validation Rules:**
- Steps cannot specify targets (targets belong to node instantiations)
- Steps cannot specify depends_on (graph structure belongs to nodes)
- Steps must have a valid primitive type (not another step)
- Step names cannot collide with primitive types
- Duplicate step names are rejected
- Step failure_policy is optional and inherited by nodes

**Step Expansion Lowering Rules:**
- When a node references a step, it's expanded to a concrete node
- Step defaults are cloned as base inputs
- Node inputs override step defaults
- Node can override step failure_policy
- Node retains its own targets and depends_on

**Input Merging Semantics:**
- Step defines default inputs
- Node can override any input
- Node inputs take precedence over step defaults
- Failure policy can be inherited or overridden

**Language Version 0.5 Enhancements:**
- Steps can reference other steps (v0.5 feature)
- Step dependency graph is built for cycle detection
- Circular dependencies are detected using DFS with cycle path reporting
- Step type resolution ensures eventual primitive types
- Recursive step expansion with memoization

**Language Version 0.6 Enhancements:**
- Parameter validation ensures parameter uniqueness and default value validity
- Parameter substitution replaces parameter references with provided values or defaults
- Compile-time determinism ensures parameter defaults are evaluated once per step definition
- Parameter shadowing detection prevents conflicts between parameter names and other identifiers

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L804-L894)
- [validate.go](file://internal/devlang/validate.go#L1198-L1222)
- [validate.go](file://internal/devlang/validate.go#L1413-L1520)
- [validate.go](file://internal/devlang/validate.go#L1669-L1711)
- [lower.go](file://internal/devlang/lower.go#L217-L248)
- [lower.go](file://internal/devlang/lower.go#L394-L479)
- [lower.go](file://internal/devlang/lower.go#L709-L870)

### For-Loop Validation and Unrolling
Language version 0.5 introduces comprehensive for-loop validation and compile-time unrolling:

**For-Loop Validation Rules:**
- For-loops are allowed with compile-time evaluation constraints
- Range must evaluate to a literal list of strings
- For-loop bodies may only contain node declarations
- Nested for-loops are not allowed
- Loop variables are substituted in node names and string inputs

**For-Loop Unrolling Rules:**
- For-loops are unrolled into concrete node definitions at compile time
- Loop variables replace placeholders in node names and string literals
- Each loop iteration produces a separate node in the final plan
- For-loop generated nodes are excluded from symbol table building

**Language Version 0.6 Enhancements:**
- Parameter substitution applies to for-loop unrolling
- Loop variables and parameter values are substituted in node names and string inputs
- Compile-time determinism ensures consistent loop expansion behavior

**Compile-Time Evaluation Constraints:**
- For-loop ranges must be evaluatable to literal lists
- Range expressions can reference let bindings
- Elements in range lists must be string literals
- For-loop bodies are statically validated before unrolling

```mermaid
flowchart TD
ForLoop["For-Loop Declaration"] --> ValidateRange["Validate Range Expression"]
ValidateRange --> CheckLet["Resolve Let Bindings"]
CheckLet --> CheckList["Check List Literal"]
CheckList --> CheckElements["Check String Literals"]
CheckElements --> ValidateBody["Validate Body Declarations"]
ValidateBody --> UnrollLoop["Unroll Loop Iterations"]
UnrollLoop --> SubstituteVars["Substitute Loop Variables"]
SubstituteVars --> SubstituteParams["Substitute Parameters"]
SubstituteParams --> GenerateNodes["Generate Concrete Nodes"]
GenerateNodes --> LowerNodes["Lower to Plan Nodes"]
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L1233-L1294)
- [validate.go](file://internal/devlang/validate.go#L1775-L1850)
- [lower.go](file://internal/devlang/lower.go#L338-L389)
- [lower.go](file://internal/devlang/lower.go#L507-L565)
- [lower.go](file://internal/devlang/lower.go#L653-L704)
- [lower.go](file://internal/devlang/lower.go#L709-L870)

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L1233-L1294)
- [validate.go](file://internal/devlang/validate.go#L1775-L1850)
- [lower.go](file://internal/devlang/lower.go#L338-L389)
- [lower.go](file://internal/devlang/lower.go#L507-L565)
- [lower.go](file://internal/devlang/lower.go#L653-L704)
- [lower.go](file://internal/devlang/lower.go#L709-L870)

### Parameter Validation and Substitution
Language version 0.6 introduces comprehensive parameter validation and substitution mechanisms:

**Parameter Validation Rules:**
- Parameter names must be unique within a step definition
- Parameter default values are type-checked independently of let bindings
- Parameter defaults are evaluated once per step definition (compile-time determinism)
- Parameter defaults are constant-folded to concrete literals
- Parameter shadowing is detected and reported with detailed error messages
- Parameter names cannot shadow step names or primitive types

**Parameter Substitution Mechanisms:**
- Parameter environment is built from node inputs and step defaults
- Parameter values replace parameter references in step inputs during expansion
- Parameter substitution applies to all expression types (identifiers, binary operations, ternary expressions, lists)
- Parameter substitution maintains type safety and expression structure
- Parameter substitution is applied recursively during nested step expansion

**Compile-Time Determinism:**
- Parameter defaults are evaluated during step validation, not during expansion
- Parameter values are substituted during step expansion, ensuring consistent behavior
- Parameter substitution preserves compile-time evaluation guarantees
- Parameter validation catches errors before runtime execution

**Parameter Shadowing Detection:**
- Parameter names are checked against step names and primitive types
- Parameter names are checked against other parameter names in the same step
- Parameter names are checked against node input keys to prevent conflicts
- Parameter shadowing errors provide clear diagnostic information

```mermaid
flowchart TD
ParamDecl["Parameter Declaration"] --> CheckUniqueness["Check Parameter Name Uniqueness"]
CheckUniqueness --> TypeCheckDefault["Type-Check Parameter Default"]
TypeCheckDefault --> EvalDefault["Evaluate Parameter Default"]
EvalDefault --> ReplaceDefault["Replace with Evaluated Literal"]
ReplaceDefault --> BuildParamEnv["Build Parameter Environment"]
BuildParamEnv --> CheckShadowing["Check Parameter Shadowing"]
CheckShadowing --> ExpandStep["Expand Step with Parameter Substitution"]
ExpandStep --> SubstituteParams["Substitute Parameters in Step Inputs"]
SubstituteParams --> MergeInputs["Merge with Node Inputs"]
MergeInputs --> LowerNode["Lower to Plan Node"]
```

**Diagram sources**
- [validate.go](file://internal/devlang/validate.go#L1669-L1711)
- [validate.go](file://internal/devlang/validate.go#L1907-L1924)
- [lower.go](file://internal/devlang/lower.go#L709-L870)
- [ast.go](file://internal/devlang/ast.go#L65-L81)

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L1669-L1711)
- [validate.go](file://internal/devlang/validate.go#L1907-L1924)
- [lower.go](file://internal/devlang/lower.go#L709-L870)
- [ast.go](file://internal/devlang/ast.go#L65-L81)

### Example Constructs: Valid vs Invalid
Valid constructs:
- A target with a string address.
- A node with type file.sync or process.exec and required inputs.
- Nodes with depends_on referencing other nodes by ID.
- Nodes with failure_policy set to one of halt, continue, rollback.
- **Language Version 0.2**: Let bindings with string, bool, or list of string literal values.
- **Language Version 0.3**: Advanced expressions including string concatenation, logical operations, equality comparisons, and ternary expressions with proper type checking.
- **Language Version 0.4**: Step definitions with proper primitive types, input merging with node overrides, and step expansion to concrete nodes.
- **Language Version 0.5**: For-loops with compile-time evaluation, nested step references with circular dependency detection, and recursive step expansion.
- **Language Version 0.6**: Step parameters with unique names, valid default values, parameter substitution, and compile-time determinism.

Invalid constructs:
- Using unsupported constructs (for, step, module) in v0.1, v0.2, v0.3, or v0.4.
- Duplicate target, node, or let bindings.
- Referencing unknown targets or nodes in depends_on.
- Using an unknown primitive type.
- Omitting required attributes for primitives (e.g., missing src or dest for file.sync, missing cmd or cwd for process.exec).
- Setting failure_policy to an invalid value.
- **Language Version 0.2**: Using identifiers in targets or let bindings with non-literal values.
- **Language Version 0.3**: Type mismatches in expressions, unresolved identifiers, and unsupported expression types.
- **Language Version 0.4**: Steps with invalid constructs (targets, depends_on), unknown step types, duplicate step names, primitive type collisions, and undefined step references.
- **Language Version 0.5**: Module constructs, invalid for-loop ranges, for-loop bodies containing non-node declarations, circular step dependencies, self-referencing steps, and nested step cycles.
- **Language Version 0.6**: Duplicate parameter names, invalid parameter default types, parameter shadowing conflicts, missing required parameters, and parameter substitution errors.

Examples in the repository:
- Valid plan examples demonstrate correct usage of targets and nodes with primitives.
- An end-to-end plan with depends_on illustrates ordering and dependency validation.
- **Language Version 0.2**: Examples show let bindings with string and list literal values.
- **Language Version 0.3**: Comprehensive examples demonstrate advanced expressions, type checking, and constant folding.
- **Language Version 0.4**: Comprehensive examples demonstrate step definitions, input merging, failure policy inheritance, and step expansion.
- **Language Version 0.5**: Comprehensive examples demonstrate for-loops with compile-time evaluation, nested step references, circular dependency detection, and recursive step expansion.
- **Language Version 0.6**: Comprehensive examples demonstrate parameter validation, parameter substitution, compile-time determinism, and parameter shadowing detection.

**Section sources**
- [plan.devops](file://plan.devops#L1-L20)
- [plan_resume.devops](file://tests/e2e/plan_resume.devops#L1-L43)
- [compile_test.go](file://internal/devlang/compile_test.go#L211-L303)
- [comprehensive.devops](file://tests/v0_3/valid/comprehensive.devops#L1-L46)
- [concat.devops](file://tests/v0_3/valid/concat.devops#L1-L15)
- [logical.devops](file://tests/v0_3/valid/logical.devops#L1-L16)
- [ternary.devops](file://tests/v0_3/valid/ternary.devops#L1-L17)
- [step_basic.devops](file://tests/v0_4/valid/step_basic.devops#L1-L17)
- [step_comprehensive.devops](file://tests/v0_4/valid/step_comprehensive.devops#L1-L48)
- [step_multiple_targets.devops](file://tests/v0_4/valid/step_multiple_targets.devops#L1-L27)
- [step_override_inputs.devops](file://tests/v0_4/valid/step_override_inputs.devops#L1-L18)
- [for_basic.devops](file://tests/v0_5/valid/for_basic.devops#L1-L21)
- [for_multiple_loops.devops](file://tests/v0_5/valid/for_multiple_loops.devops#L1-L21)
- [for_with_let_range.devops](file://tests/v0_5/valid/for_with_let_range.devops#L1-L18)
- [for_with_lets.devops](file://tests/v0_5/valid/for_with_lets.devops#L1-L21)
- [nested_step_basic.devops](file://tests/v0_5/valid/nested_step_basic.devops#L1-L21)
- [nested_step_deep.devops](file://tests/v0_5/valid/nested_step_deep.devops#L1-L26)
- [nested_step_override.devops](file://tests/v0_5/valid/nested_step_override.devops#L1-L21)
- [param_basic.devops](file://tests/v0_6/valid/param_basic.devops#L1-L18)
- [param_required.devops](file://tests/v0_6/valid/param_required.devops#L1-L19)
- [param_duplicate.devops](file://tests/v0_6/invalid/param_duplicate.devops#L1-L19)
- [param_missing_required.devops](file://tests/v0_6/invalid/param_missing_required.devops#L1-L17)
- [param_with_default.devops](file://tests/v0_6/hash_stability/param_with_default.devops#L1-L18)
- [param_manual_expansion.devops](file://tests/v0_6/hash_stability/param_manual_expansion.devops#L1-L18)

## Comprehensive Validation Tests

### Test Coverage Overview
The validation system includes comprehensive test coverage for all language versions with enhanced coverage for v0.6:

#### Language Version 0.1 Test Coverage
- Unknown target detection
- Duplicate node detection
- Invalid failure policy validation
- Unsupported construct testing (let, for, step, module)

#### Language Version 0.2 Test Coverage
- Enhanced unsupported construct testing
- Let binding validation with literal type restrictions
- Duplicate let binding detection
- Let binding in targets rejection
- Complex let binding scenarios

#### Language Version 0.3 Test Coverage
- Advanced expression evaluation testing
- Type checking validation for all expression types
- Constant folding verification
- Error reporting for type mismatches
- Unresolved identifier handling
- Hash stability testing for expression evaluation

#### Language Version 0.4 Test Coverage
- Step definition validation testing
- Duplicate step detection
- Primitive type collision prevention
- Step expansion lowering rules
- Input merging and override semantics
- Failure policy inheritance
- Step reference resolution
- Comprehensive error reporting for step-related failures

#### Language Version 0.5 Test Coverage
- For-loop validation testing
- Compile-time evaluation constraints
- Nested step validation testing
- Circular dependency detection
- For-loop unrolling behavior
- Recursive step expansion
- Mixed construct scenarios
- Comprehensive error reporting for v0.5-specific failures

#### Language Version 0.6 Test Coverage
- Parameter validation testing
- Parameter uniqueness validation
- Parameter default type checking
- Parameter shadowing detection
- Parameter substitution behavior
- Required parameter validation
- Compile-time determinism verification
- Mixed construct scenarios with parameters
- Comprehensive error reporting for v0.6-specific failures

```mermaid
flowchart TD
TestSuite["Validation Test Suite"] --> V01Tests["v0.1 Tests"]
TestSuite --> V02Tests["v0.2 Tests"]
TestSuite --> V03Tests["v0.3 Tests"]
TestSuite --> V04Tests["v0.4 Tests"]
TestSuite --> V05Tests["v0.5 Tests"]
TestSuite --> V06Tests["v0.6 Tests"]
V01Tests --> UnknownTarget["Unknown Target Tests"]
V01Tests --> DuplicateNode["Duplicate Node Tests"]
V01Tests --> InvalidPolicy["Invalid Failure Policy Tests"]
V01Tests --> UnsupportedConstruct["Unsupported Construct Tests"]
V02Tests --> EnhancedUnsupported["Enhanced Unsupported Construct Tests"]
V02Tests --> LetBindings["Let Binding Tests"]
V02Tests --> DuplicateLet["Duplicate Let Tests"]
V02Tests --> LetInTargets["Let In Targets Tests"]
V03Tests --> AdvancedExpr["Advanced Expression Tests"]
V03Tests --> TypeChecking["Type Checking Tests"]
V03Tests --> ConstFolding["Constant Folding Tests"]
V03Tests --> ErrorReporting["Error Reporting Tests"]
V03Tests --> HashStability["Hash Stability Tests"]
V04Tests --> StepDefs["Step Definition Tests"]
V04Tests --> DuplicateStep["Duplicate Step Tests"]
V04Tests --> PrimitiveCollision["Primitive Collision Tests"]
V04Tests --> StepExpansion["Step Expansion Tests"]
V04Tests --> InputMerging["Input Merging Tests"]
V04Tests --> FailurePolicyInherit["Failure Policy Inheritance Tests"]
V04Tests --> StepResolution["Step Resolution Tests"]
V05Tests --> ForLoops["For-Loop Tests"]
V05Tests --> ForRangeValidation["For-Loop Range Validation Tests"]
V05Tests --> ForBodyValidation["For-Loop Body Validation Tests"]
V05Tests --> NestedSteps["Nested Step Tests"]
V05Tests --> CycleDetection["Circular Dependency Detection Tests"]
V05Tests --> StepUnrolling["Step Unrolling Tests"]
V05Tests --> MixedConstructs["Mixed Construct Tests"]
V06Tests --> Params["Parameter Tests"]
V06Tests --> ParamUniqueness["Parameter Uniqueness Tests"]
V06Tests --> ParamDefaults["Parameter Default Tests"]
V06Tests --> ParamShadowing["Parameter Shadowing Tests"]
V06Tests --> ParamSubstitution["Parameter Substitution Tests"]
V06Tests --> ParamRequired["Required Parameter Tests"]
V06Tests --> ParamDeterminism["Parameter Determinism Tests"]
V06Tests --> ParamMixed["Mixed Parameter Tests"]
UnknownTarget --> UTExpected["Expected 'unknown target' errors"]
DuplicateNode --> DNExpected["Expected 'duplicate node' errors"]
InvalidPolicy --> IPExpected["Expected 'invalid failure_policy' errors"]
UnsupportedConstruct --> UCExpected["Expected 'not supported' errors"]
EnhancedUnsupported --> EUCExpected["Expected 'not supported' errors"]
LetBindings --> LBExpected["Expected literal type validation"]
DuplicateLet --> DLExpected["Expected 'duplicate let' errors"]
LetInTargets --> LITExpected["Expected 'cannot be used in targets' errors"]
AdvancedExpr --> AEExpected["Expected expression evaluation"]
TypeChecking --> TCExpected["Expected type checking"]
ConstFolding --> CFExpected["Expected constant folding"]
ErrorReporting --> ERExpected["Expected detailed error messages"]
HashStability --> HSExpected["Expected stable hash values"]
StepDefs --> SDExpected["Expected step definition validation"]
DuplicateStep --> DSExpected["Expected 'duplicate step' errors"]
PrimitiveCollision --> PCExpected["Expected 'conflicts with built-in primitive' errors"]
StepExpansion --> SEExpected["Expected step expansion behavior"]
InputMerging --> IMExpected["Expected input merging semantics"]
FailurePolicyInherit --> FPIExpected["Expected failure policy inheritance"]
StepResolution --> SRExpected["Expected step reference resolution"]
ForLoops --> FTExpected["Expected for-loop validation"]
ForRangeValidation --> FRVExpected["Expected range validation"]
ForBodyValidation --> FBVExpected["Expected body validation"]
NestedSteps --> NSTExpected["Expected nested step validation"]
CycleDetection --> CDExpected["Expected cycle detection"]
StepUnrolling --> SUExpected["Expected step unrolling behavior"]
MixedConstructs --> MCExpected["Expected mixed construct handling"]
Params --> PExpected["Expected parameter validation"]
ParamUniqueness --> PUExpected["Expected parameter uniqueness validation"]
ParamDefaults --> PDExpected["Expected parameter default type checking"]
ParamShadowing --> PSExpected["Expected parameter shadowing detection"]
ParamSubstitution --> PSEnexpected["Expected parameter substitution behavior"]
ParamRequired --> PRExpected["Expected required parameter validation"]
ParamDeterminism --> PDTExpected["Expected compile-time determinism"]
ParamMixed --> PMExpected["Expected mixed parameter handling"]
```

**Diagram sources**
- [compile_test.go](file://internal/devlang/compile_test.go#L118-L429)

**Section sources**
- [compile_test.go](file://internal/devlang/compile_test.go#L118-L429)

### Test Case Analysis

#### Language Version 0.1 Test Cases
The v0.1 test suite verifies:
- Unknown target validation: Node declaration with reference to non-existent target "prod" produces "unknown target" error.
- Duplicate node detection: Two node declarations with identical names "dup" produce "duplicate node" error.
- Invalid failure policy: Node with unsupported failure_policy "fast" produces "invalid failure_policy" error.
- Unsupported construct testing: Parsing of unsupported constructs (let, for, step, module) produces immediate semantic errors.

#### Language Version 0.2 Test Cases
The v0.2 test suite expands coverage:
- Enhanced unsupported construct testing: Validates rejection of for, step, and module constructs.
- Let binding validation: Tests string literals, bool literals, and list literals for let bindings.
- Duplicate let detection: Two let declarations with identical names "x" produce "duplicate let" error.
- Literal type restrictions: Identifiers and non-string list elements in let values produce "must be a string, bool, or list of string literals" errors.
- Let binding in targets: Using let bindings in targets produces "cannot be used in targets" error.
- Complex scenarios: String and list let bindings successfully compile to plans with resolved values.

#### Language Version 0.3 Test Cases
The v0.3 test suite provides comprehensive coverage:
- **Advanced expression evaluation**: String concatenation, logical operations, equality comparisons, and ternary expressions.
- **Type checking validation**: Ensures proper type inference and enforcement for all expression types.
- **Constant folding verification**: Confirms expressions are evaluated to concrete literals at compile time.
- **Error reporting**: Provides detailed error messages for type mismatches, unresolved identifiers, and unsupported operations.
- **Hash stability testing**: Verifies that expression evaluation produces consistent hash values across compilations.

#### Language Version 0.4 Test Cases
The v0.4 test suite provides comprehensive coverage:
- **Step definition validation**: Proper step definitions with valid primitive types and required attributes.
- **Duplicate step detection**: Prevents naming conflicts between steps and other steps or primitives.
- **Primitive type collision prevention**: Ensures step names don't collide with built-in primitive types.
- **Step expansion lowering**: Transforms step references into concrete node definitions with merged inputs.
- **Input merging and override semantics**: Allows steps to define defaults while enabling node-level overrides.
- **Failure policy inheritance**: Enables steps to define default failure policies with node-level overrides.
- **Step reference resolution**: Validates that all step references resolve to defined step definitions.
- **Error reporting**: Provides detailed error messages for step-related validation failures.

#### Language Version 0.5 Test Cases
The v0.5 test suite provides comprehensive coverage:
- **For-loop validation**: Proper for-loop declarations with compile-time evaluation constraints.
- **For-loop range validation**: Ensures ranges evaluate to literal lists of strings.
- **For-loop body validation**: Restricts for-loop bodies to contain only node declarations.
- **Nested step validation**: Allows steps to reference other steps with proper validation.
- **Circular dependency detection**: Detects and reports circular step dependencies with cycle path information.
- **Step unrolling**: Expands nested steps recursively into concrete node definitions.
- **For-loop unrolling**: Unrolls loops into concrete node definitions with variable substitution.
- **Mixed construct scenarios**: Handles combinations of for-loops, nested steps, and traditional constructs.
- **Error reporting**: Provides detailed error messages for v0.5-specific validation failures.

#### Language Version 0.6 Test Cases
The v0.6 test suite provides comprehensive coverage:
- **Parameter validation**: Proper parameter declarations with unique names and valid default values.
- **Parameter uniqueness validation**: Prevents duplicate parameter names within step definitions.
- **Parameter default type checking**: Ensures parameter defaults are type-checked independently of let bindings.
- **Parameter shadowing detection**: Prevents conflicts between parameter names and step/primitive names.
- **Parameter substitution behavior**: Validates that parameter values replace parameter references during step expansion.
- **Required parameter validation**: Ensures nodes provide values for required parameters without defaults.
- **Compile-time determinism verification**: Confirms parameter defaults are evaluated once per step definition.
- **Mixed construct scenarios with parameters**: Handles combinations of parameters, steps, for-loops, and traditional constructs.
- **Error reporting**: Provides detailed error messages for v0.6-specific validation failures including parameter-related errors.

**Section sources**
- [compile_test.go](file://internal/devlang/compile_test.go#L118-L429)
- [comprehensive.devops](file://tests/v0_3/valid/comprehensive.devops#L1-L46)
- [concat.devops](file://tests/v0_3/valid/concat.devops#L1-L15)
- [logical.devops](file://tests/v0_3/valid/logical.devops#L1-L16)
- [ternary.devops](file://tests/v0_3/valid/ternary.devops#L1-L17)
- [type_mismatch.devops](file://tests/v0_3/invalid/type_mismatch.devops#L1-L13)
- [unresolved_var.devops](file://tests/v0_3/invalid/unresolved_var.devops#L1-L13)
- [expr_version.devops](file://tests/v0_3/hash_stability/expr_version.devops#L1-L13)
- [literal_version.devops](file://tests/v0_3/hash_stability/literal_version.devops#L1-L13)
- [step_basic.devops](file://tests/v0_4/valid/step_basic.devops#L1-L17)
- [step_comprehensive.devops](file://tests/v0_4/valid/step_comprehensive.devops#L1-L48)
- [step_multiple_targets.devops](file://tests/v0_4/valid/step_multiple_targets.devops#L1-L27)
- [step_override_inputs.devops](file://tests/v0_4/valid/step_override_inputs.devops#L1-L18)
- [step_duplicate.devops](file://tests/v0_4/invalid/step_duplicate.devops#L1-L23)
- [step_primitive_collision.devops](file://tests/v0_4/invalid/step_primitive_collision.devops#L1-L15)
- [step_undefined.devops](file://tests/v0_4/invalid/step_undefined.devops#L1-L10)
- [step_unknown_primitive.devops](file://tests/v0_4/invalid/step_unknown_primitive.devops#L1-L15)
- [step_with_depends_on.devops](file://tests/v0_4/invalid/step_with_depends_on.devops#L1-L17)
- [step_with_targets.devops](file://tests/v0_4/invalid/step_with_targets.devops#L1-L21)
- [for_basic.devops](file://tests/v0_5/valid/for_basic.devops#L1-L21)
- [for_multiple_loops.devops](file://tests/v0_5/valid/for_multiple_loops.devops#L1-L21)
- [for_with_let_range.devops](file://tests/v0_5/valid/for_with_let_range.devops#L1-L18)
- [for_with_lets.devops](file://tests/v0_5/valid/for_with_lets.devops#L1-L21)
- [nested_step_basic.devops](file://tests/v0_5/valid/nested_step_basic.devops#L1-L21)
- [nested_step_deep.devops](file://tests/v0_5/valid/nested_step_deep.devops#L1-L26)
- [nested_step_override.devops](file://tests/v0_5/valid/nested_step_override.devops#L1-L21)
- [for_non_list_range.devops](file://tests/v0_5/invalid/for_non_list_range.devops#L1-L16)
- [nested_step_cycle_direct.devops](file://tests/v0_5/invalid/nested_step_cycle_direct.devops#L1-L21)
- [nested_step_cycle_indirect.devops](file://tests/v0_5/invalid/nested_step_cycle_indirect.devops#L1-L23)
- [nested_step_self_reference.devops](file://tests/v0_5/invalid/nested_step_self_reference.devops#L1-L16)
- [param_basic.devops](file://tests/v0_6/valid/param_basic.devops#L1-L18)
- [param_required.devops](file://tests/v0_6/valid/param_required.devops#L1-L19)
- [param_duplicate.devops](file://tests/v0_6/invalid/param_duplicate.devops#L1-L19)
- [param_missing_required.devops](file://tests/v0_6/invalid/param_missing_required.devops#L1-L17)
- [param_with_default.devops](file://tests/v0_6/hash_stability/param_with_default.devops#L1-L18)
- [param_manual_expansion.devops](file://tests/v0_6/hash_stability/param_manual_expansion.devops#L1-L18)

## Dependency Analysis
The validation pipeline depends on:
- Lexer and Parser for correct AST construction.
- Semantic Validator (v0.1, v0.2, v0.3, v0.4, v0.5, and v0.6) for language-version-specific checks with v0.6 adding parameter validation and step parameter support.
- Type Checker (v0.3) for comprehensive type inference and validation.
- Expression Evaluator (v0.3) for constant folding and expression resolution.
- Lowerer (v0.1, v0.2, v0.3, v0.4, v0.5, and v0.6) for converting AST to plan IR with enhanced let environment support, step expansion, parameter substitution, and for-loop unrolling.
- Plan Validator for structural correctness.

```mermaid
graph LR
LEX["Lexer"] --> PARSE["Parser"]
PARSE --> SEMVAL01["Semantic Validator v0.1"]
PARSE --> SEMVAL02["Semantic Validator v0.2"]
PARSE --> SEMVAL03["Semantic Validator v0.3"]
PARSE --> SEMVAL04["Semantic Validator v0.4"]
PARSE --> SEMVAL05["Semantic Validator v0.5"]
PARSE --> SEMVAL06["Semantic Validator v0.6"]
SEMVAL03 --> TYPECHECK["Type Checker"]
SEMVAL03 --> EXPRVAL["Expression Evaluator"]
SEMVAL01 --> LOWER01["Lowerer v0.1"]
SEMVAL02 --> LOWER02["Lowerer v0.2"]
SEMVAL03 --> LOWER03["Lowerer v0.3"]
SEMVAL04 --> LOWER04["Lowerer v0.4"]
SEMVAL05 --> LOWER05["Lowerer v0.5"]
SEMVAL06 --> LOWER06["Lowerer v0.6"]
LOWER01 --> IRVAL["Plan Validator"]
LOWER02 --> IRVAL
LOWER03 --> IRVAL
LOWER04 --> IRVAL
LOWER05 --> IRVAL
LOWER06 --> IRVAL
```

**Diagram sources**
- [lexer.go](file://internal/devlang/lexer.go#L60-L100)
- [parser.go](file://internal/devlang/parser.go#L28-L78)
- [validate.go](file://internal/devlang/validate.go#L23-L194)
- [validate.go](file://internal/devlang/validate.go#L196-L315)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)
- [lower.go](file://internal/devlang/lower.go#L10-L870)
- [validate.go](file://internal/plan/validate.go#L7-L94)

**Section sources**
- [lexer.go](file://internal/devlang/lexer.go#L1-L247)
- [parser.go](file://internal/devlang/parser.go#L1-L495)
- [validate.go](file://internal/devlang/validate.go#L1-L2011)
- [lower.go](file://internal/devlang/lower.go#L1-L870)
- [validate.go](file://internal/plan/validate.go#L1-L95)

## Performance Considerations
- Validation is linear in the number of declarations and nodes.
- Duplicate detection and cross-reference checks use maps for O(1) average-time lookups.
- Early rejection of unsupported constructs avoids unnecessary downstream work.
- IR validation mirrors AST checks to catch issues earlier and reduce runtime surprises.
- **Language Version 0.2**: Let environment collection uses O(n) time where n is the number of let declarations.
- **Language Version 0.3**: Type checking and expression evaluation add computational overhead but provide compile-time safety guarantees.
- **Language Version 0.3**: Constant folding eliminates runtime computation for expressions, improving performance at execution time.
- **Language Version 0.4**: Step collection and validation adds O(s) time where s is the number of step definitions.
- **Language Version 0.4**: Step expansion lowers O(n) nodes to O(n+s) nodes in the final plan, with input merging complexity proportional to input count.
- **Language Version 0.4**: Input merging and override resolution occurs during lowering, avoiding runtime computation.
- **Language Version 0.5**: For-loop validation adds O(f) time where f is the number of for-loops, with O(f*m) complexity for unrolling where m is average loop iterations.
- **Language Version 0.5**: Step dependency graph construction uses O(s) time for adjacency list creation.
- **Language Version 0.5**: Circular dependency detection uses O(s) time with DFS traversal.
- **Language Version 0.5**: Recursive step expansion uses memoization to avoid redundant expansions, with O(depth) recursion per step.
- **Language Version 0.5**: For-loop unrolling generates O(f*m) additional nodes in the final plan.
- **Language Version 0.6**: Parameter validation adds O(p) time where p is the total number of parameters across all steps.
- **Language Version 0.6**: Parameter default type checking and evaluation adds compile-time overhead but ensures runtime safety.
- **Language Version 0.6**: Parameter substitution during step expansion adds O(p) time per step expansion, with recursive parameter substitution complexity.
- **Language Version 0.6**: Compile-time determinism ensures parameter defaults are evaluated once, avoiding repeated computation during expansion.
- **Language Version 0.6**: Parameter shadowing detection uses O(p) time with name collision checks.
- **Language Version 0.6**: Parameter environment building uses O(p) time for parameter value resolution.

## Troubleshooting Guide
Common validation failures and remedies:

### Language Version 0.1 Issues
- Unsupported construct in v0.1:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs or upgrade language version if applicable.
  - Reference: [validate.go](file://internal/devlang/validate.go#L200-L227)
- Duplicate target or node:
  - Symptom: Error indicating duplicate declaration.
  - Fix: Rename one of the conflicting declarations.
  - Reference: [validate.go](file://internal/devlang/validate.go#L234-L261)
- Unknown target or node reference:
  - Symptom: Error indicating unknown target or node in depends_on.
  - Fix: Define the referenced target/node or correct the name.
  - Reference: [validate.go](file://internal/devlang/validate.go#L264-L285)
- Unknown primitive type:
  - Symptom: Error indicating unknown primitive type.
  - Fix: Use a supported primitive type.
  - Reference: [validate.go](file://internal/devlang/validate.go#L287-L297)
- Invalid failure_policy:
  - Symptom: Error indicating invalid failure policy.
  - Fix: Set failure_policy to one of halt, continue, rollback.
  - Reference: [validate.go](file://internal/devlang/validate.go#L299-L309)
- Primitive input constraints:
  - file.sync requires src and dest as string literals.
  - process.exec requires cmd as a non-empty list of string literals and cwd as a string literal.
  - Fix: Provide correct types and values for required attributes.
  - References: [validate.go](file://internal/devlang/validate.go#L317-L382)

### Language Version 0.2 Issues
- Unsupported construct in v0.2:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs (for, step, module) or use supported alternatives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L28-L50)
- Duplicate let binding:
  - Symptom: Error indicating duplicate let declaration.
  - Fix: Rename one of the conflicting let bindings.
  - Reference: [validate.go](file://internal/devlang/validate.go#L63-L70)
- Let binding literal type restrictions:
  - Symptom: Error indicating invalid let value type.
  - Fix: Use string, bool, or list of string literals for let values.
  - Reference: [validate.go](file://internal/devlang/validate.go#L72-L91)
- Let binding in targets:
  - Symptom: Error indicating let binding cannot be used in targets.
  - Fix: Replace let binding with direct target reference.
  - Reference: [validate.go](file://internal/devlang/validate.go#L131-L137)
- Identifier as value in v0.2:
  - Symptom: Internal error indicating identifiers cannot be lowered as values.
  - Fix: Use string literals or lists of string literals for primitive inputs.
  - References: [lower.go](file://internal/devlang/lower.go#L166-L174)

### Language Version 0.3 Issues
- Unsupported construct in v0.3:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs (for, step, module) or use supported alternatives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L498-L520)
- Duplicate let binding:
  - Symptom: Error indicating duplicate let declaration.
  - Fix: Rename one of the conflicting let bindings.
  - Reference: [validate.go](file://internal/devlang/validate.go#L533-L540)
- Let binding expression type checking:
  - Symptom: Error indicating type mismatch in expression.
  - Fix: Ensure all operands have compatible types for the operation.
  - Reference: [types.go](file://internal/devlang/types.go#L86-L142)
- Expression evaluation errors:
  - Symptom: Error indicating unsupported expression type or unresolved identifier.
  - Fix: Use supported expression constructs and ensure all identifiers are defined.
  - References: [eval.go](file://internal/devlang/eval.go#L144-L180)
- Constant folding failures:
  - Symptom: Error indicating expression cannot be evaluated to a literal.
  - Fix: Ensure expressions contain only literals and supported operations.
  - References: [eval.go](file://internal/devlang/eval.go#L174-L180)
- Type mismatch in ternary expressions:
  - Symptom: Error indicating branches have different types.
  - Fix: Ensure both branches of ternary expressions have the same type.
  - Reference: [types.go](file://internal/devlang/types.go#L166-L172)
- List comparison not supported:
  - Symptom: Error indicating comparison of string lists is not allowed.
  - Fix: Compare individual string elements instead of entire lists.
  - Reference: [types.go](file://internal/devlang/types.go#L126-L133)

### Language Version 0.4 Issues
- Unsupported construct in v0.4:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs (for, module) or use supported alternatives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L729-L744)
- Duplicate let binding:
  - Symptom: Error indicating duplicate let declaration.
  - Fix: Rename one of the conflicting let bindings.
  - Reference: [validate.go](file://internal/devlang/validate.go#L758-L764)
- Step definition validation errors:
  - Symptom: Error indicating step cannot specify targets or depends_on.
  - Fix: Remove targets or depends_on from step definitions; define them in node instantiations.
  - References: [validate.go](file://internal/devlang/validate.go#L833-L849)
- Unknown step type:
  - Symptom: Error indicating step references unknown step or primitive.
  - Fix: Define the referenced step or use a valid primitive type.
  - References: [validate.go](file://internal/devlang/validate.go#L861-L878)
- Duplicate step name:
  - Symptom: Error indicating duplicate step name.
  - Fix: Rename one of the conflicting step definitions.
  - Reference: [validate.go](file://internal/devlang/validate.go#L811-L818)
- Primitive type collision:
  - Symptom: Error indicating step name conflicts with built-in primitive.
  - Fix: Choose a different step name that doesn't collide with primitives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L821-L828)
- Undefined step reference:
  - Symptom: Error indicating node references non-existent step.
  - Fix: Define the referenced step or correct the step name.
  - Reference: [validate.go](file://internal/devlang/validate.go#L946-L952)
- Step expansion errors:
  - Symptom: Error during step expansion lowering.
  - Fix: Ensure step definitions are valid and all references resolve correctly.
  - References: [lower.go](file://internal/devlang/lower.go#L217-L248)

### Language Version 0.5 Issues
- Unsupported construct in v0.5:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs (module) or use supported alternatives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1065-L1075)
- Duplicate let binding:
  - Symptom: Error indicating duplicate let declaration.
  - Fix: Rename one of the conflicting let bindings.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1088-L1095)
- For-loop range validation errors:
  - Symptom: Error indicating for-loop range must evaluate to a literal list of strings.
  - Fix: Ensure range expression evaluates to a list literal with string elements.
  - References: [validate.go](file://internal/devlang/validate.go#L1244-L1262)
- For-loop body validation errors:
  - Symptom: Error indicating for-loop body may only contain node declarations.
  - Fix: Remove step or nested for-loop declarations from for-loop body.
  - References: [validate.go](file://internal/devlang/validate.go#L1281-L1293)
- Circular step dependency:
  - Symptom: Error indicating circular step dependency detected with cycle path.
  - Fix: Break the cycle by removing one of the dependencies in the cycle.
  - References: [validate.go](file://internal/devlang/validate.go#L1200-L1211)
- Self-referencing step:
  - Symptom: Error indicating step references itself.
  - Fix: Change step type to reference a different step or primitive.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1510-L1516)
- Nested step cycle detection:
  - Symptom: Error indicating nested step dependency cycle.
  - Fix: Ensure step dependencies form a directed acyclic graph.
  - References: [validate.go](file://internal/devlang/validate.go#L1430-L1478)
- For-loop unrolling errors:
  - Symptom: Error during for-loop unrolling or variable substitution.
  - Fix: Ensure for-loop ranges are valid and variable substitutions succeed.
  - References: [lower.go](file://internal/devlang/lower.go#L338-L389)

### Language Version 0.6 Issues
- Unsupported construct in v0.6:
  - Symptom: Error indicating the construct is not supported.
  - Fix: Remove unsupported constructs (module) or use supported alternatives.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1573-L1582)
- Duplicate let binding:
  - Symptom: Error indicating duplicate let declaration.
  - Fix: Rename one of the conflicting let bindings.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1596-L1603)
- Parameter validation errors:
  - Duplicate parameter name:
    - Symptom: Error indicating duplicate parameter in step definition.
    - Fix: Rename one of the conflicting parameters.
    - Reference: [validate.go](file://internal/devlang/validate.go#L1673-L1680)
  - Parameter default type error:
    - Symptom: Error indicating parameter default type mismatch.
    - Fix: Ensure parameter default matches the expected type.
    - Reference: [validate.go](file://internal/devlang/validate.go#L1687-L1695)
  - Parameter default evaluation error:
    - Symptom: Error indicating parameter default cannot be evaluated.
    - Fix: Ensure parameter default is a constant expression.
    - Reference: [validate.go](file://internal/devlang/validate.go#L1698-L1706)
  - Parameter shadowing conflict:
    - Symptom: Error indicating parameter shadows step or primitive name.
    - Fix: Choose a different parameter name that doesn't conflict.
    - Reference: [validate.go](file://internal/devlang/validate.go#L1685-L1686)
- Required parameter validation:
  - Symptom: Error indicating missing required parameter for step.
  - Fix: Provide a value for the required parameter in the node declaration.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1916-L1923)
- Parameter substitution errors:
  - Symptom: Error during parameter substitution in step expansion.
  - Fix: Ensure parameter values are valid and compatible with step inputs.
  - References: [lower.go](file://internal/devlang/lower.go#L725-L742)
- Compile-time determinism violations:
  - Symptom: Error indicating parameter default cannot be evaluated at compile time.
  - Fix: Ensure parameter defaults are constant expressions without external dependencies.
  - Reference: [validate.go](file://internal/devlang/validate.go#L1684-L1685)

### IR-level Errors
- Missing plan fields, targets, or nodes; missing target id/address; missing node id/type/targets; unknown depends_on or when.node; invalid failure_policy; missing or invalid primitive inputs.
- Fix: Ensure all required fields are present and valid.
- References: [validate.go](file://internal/plan/validate.go#L7-L94), [schema.go](file://internal/plan/schema.go#L12-L39)

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L28-L382)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [validate.go](file://internal/plan/validate.go#L7-L94)
- [lower.go](file://internal/devlang/lower.go#L150-L179)
- [schema.go](file://internal/plan/schema.go#L12-L39)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)

## Conclusion
The .devops language enforces strict validation at both the language and IR levels across multiple language versions. Language-level checks prevent unsupported constructs and enforce scoping and primitive constraints, while IR-level checks ensure structural correctness and consistency. The enhanced validation system now supports language version 0.6 with comprehensive parameter validation, parameter uniqueness checks, default value validation, parameter shadowing detection, and compile-time determinism enforcement. The comprehensive test suite provides extensive coverage for common error scenarios across all language versions, enabling developers to quickly identify and resolve validation issues. Together, these validations provide strong safety guarantees for runtime execution by catching errors early and preventing malformed plans from reaching the orchestrator.

## Appendices

### Relationship Between Validation Rules and Runtime Safety
- Early detection of unsupported constructs prevents undefined behavior at runtime.
- Duplicate detection and cross-reference checks ensure deterministic execution order and avoid ambiguous references.
- Primitive input validation ensures that runtime primitives receive the expected types and values, reducing failures during node execution.
- IR validation ensures the plan is self-consistent, preventing crashes due to missing or inconsistent metadata.
- **Language Version 0.2**: Let binding validation ensures compile-time safety for dynamic configuration values while maintaining runtime reliability.
- **Language Version 0.3**: Advanced expression evaluation with type checking and constant folding eliminates runtime computation errors and improves performance by pre-computing values at compile time.
- **Language Version 0.4**: Step definition validation and expansion ensures consistent behavior across node instantiations, while input merging and override semantics provide flexible configuration management without runtime overhead.
- **Language Version 0.5**: For-loop validation and compile-time unrolling eliminate runtime loop overhead and ensure predictable node generation, while nested step validation with circular dependency detection prevents infinite recursion and ensures step hierarchies resolve to primitive types.
- **Language Version 0.5**: Recursive step expansion with memoization ensures efficient compilation of complex step hierarchies without redundant computations.
- **Language Version 0.5**: For-loop unrolling with variable substitution ensures that loop-generated nodes have concrete values at compile time, eliminating runtime string manipulation overhead.
- **Language Version 0.6**: Parameter validation ensures compile-time safety for step parameters, while parameter uniqueness and default value validation prevent runtime parameter resolution issues.
- **Language Version 0.6**: Parameter substitution during step expansion ensures that parameter values are resolved consistently and efficiently, without runtime computation overhead.
- **Language Version 0.6**: Compile-time determinism for parameter defaults ensures predictable behavior and prevents inconsistent parameter values across different executions.
- **Language Version 0.6**: Parameter shadowing detection prevents naming conflicts that could lead to unexpected parameter resolution and runtime errors.
- **Language Version 0.6**: Required parameter validation ensures that all necessary parameters are provided, preventing runtime failures due to missing parameter values.
- **Language Version 0.6**: Parameter substitution maintains type safety and expression integrity throughout the compilation process, ensuring runtime safety guarantees.

**Section sources**
- [validate.go](file://internal/devlang/validate.go#L23-L382)
- [validate.go](file://internal/devlang/validate.go#L493-L677)
- [validate.go](file://internal/devlang/validate.go#L717-L1011)
- [validate.go](file://internal/devlang/validate.go#L1052-L1558)
- [validate.go](file://internal/devlang/validate.go#L1560-L1757)
- [validate.go](file://internal/plan/validate.go#L7-L94)
- [lower.go](file://internal/devlang/lower.go#L92-L870)
- [types.go](file://internal/devlang/types.go#L27-L182)
- [eval.go](file://internal/devlang/eval.go#L5-L181)