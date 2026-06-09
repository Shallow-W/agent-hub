# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

<!--
Document your project's database conventions here.

Questions to answer:
- What ORM/query library do you use?
- How are migrations managed?
- What are the naming conventions for tables/columns?
- How do you handle transactions?
-->

(To be filled by the team)

---

## Query Patterns

<!-- How should queries be written? Batch operations? -->

- For optional UUID columns populated from string parameters, cast after
  `NULLIF`: `NULLIF($n, '')::uuid`. Without the explicit cast, PostgreSQL can
  infer the expression as `text` and fail inserts with SQLSTATE `42804`.

---

## Migrations

<!-- How to create and run migrations -->

(To be filled by the team)

---

## Naming Conventions

<!-- Table names, column names, index names -->

(To be filled by the team)

---

## Common Mistakes

<!-- Database-related mistakes your team has made -->

- `NULLIF($n, '')` is not enough for UUID columns. Use
  `NULLIF($n, '')::uuid`, especially for optional reply anchors such as
  `source_message_id` and `dispatch_message_id`.
