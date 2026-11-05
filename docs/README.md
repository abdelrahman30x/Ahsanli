# AHSANLY documentation

This directory is the canonical, English, section-based documentation for the current AHSANLY project.

## Edit the documentation

Edit the numbered Markdown sections. Keep dated operational observations in `11_CURRENT_STATE_LIMITS.md` and avoid secrets or real environment values.

## Build the standalone AI handoff

From any directory, with Python 3:

```bash
python /path/to/ahsanly/docs/build_docs.py
```

Or from the repository root:

```bash
python docs/build_docs.py
```

This generates `AHSANLY_AI_GUIDE.md` at the repository root. That single file is suitable for downloading and giving to another AI.

To create a copy at a custom location:

```bash
python docs/build_docs.py --output /path/to/AHSANLY_AI_GUIDE.md
```

To verify that the committed guide is current:

```bash
python docs/build_docs.py --check
```

The build is deterministic: no timestamp, local path, hostname-specific temporary value, or random ID is inserted. CI runs `--check`, so commit the regenerated file whenever a section changes.

## Section map

1. Product identity and business rules.
2. Current architecture and repository map.
3. Public frontend, console UI, and localization.
4. HTTP API reference.
5. PostgreSQL data and domain model.
6. Offer evaluation, editorial review, and Telegram pipeline.
7. Security and operational invariants.
8. Development and testing guide.
9. GitHub deployment, server updates, and rollback.
10. Safe extension and modification recipes.
11. Current state, limitations, and discrepancies.
12. AI handoff and review checklist.
