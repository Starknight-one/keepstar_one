---
name: audit-copy
description: Audit landing page copy using the Infostyle method. Analyzes text for value judgments, weak headlines, abstractions, missing benefits, stop words, structure issues, and manipulation. Use when reviewing or improving page copy.
allowed-tools: Read, Grep, Glob, Agent
argument-hint: [file-path or page-name]
effort: high
---

# Copy Audit (Infostyle Method)

You are auditing landing page copy using the methodology from the guide below. Follow these steps precisely.

## Guide

See the full methodology: [landing-audit-guide-en.md](landing-audit-guide-en.md)

## How to run

1. **Determine scope.** If `$ARGUMENTS` is provided, audit that specific file or page. If not, audit the main landing page by reading all section components from `src/components/` (Hero, IconStrip, FeatureRows, UseCases, ProblemSection, HowItWorks, Stats, Pricing, FinalCTA) plus Header and Footer.

2. **Extract all copy.** Read each component file. Extract every user-visible string: headings, subheadings, body text, button labels, badges, list items, captions.

3. **Run all 7 checklists** from the guide against every text block:
   - **1. Value Judgments -> Facts**: empty adjectives, unsubstantiated claims, vague quantities
   - **2. Headlines**: descriptive vs transitive, autonomous readability
   - **3. Sensory Experience**: abstractions, jargon, missing scenarios
   - **4. Benefits vs Features**: feature-only lists, generic benefits, reader-addressed
   - **5. Stop Words**: filler intros, bureaucratic language, nominalizations, cliches
   - **6. Structure**: hero answers 3 questions, CTA specificity, duplication
   - **7. Honesty**: fake urgency, manipulative questions, unverifiable claims

4. **For every issue found**, output:
   ```
   Block: [section name]
   Issue: [brief description]
   Before: "...original text..."
   After: "...suggested revision..."
   Why: [principle reference]
   ```

5. **Produce the summary report** in the format from the guide:
   - Table: issues per category
   - Critical issues (fix first)
   - Medium issues
   - Minor notes
   - What's already good

## Rules

- **Don't rewrite everything.** Find issues, suggest targeted fixes. The author decides.
- **Preserve brand voice.** The landing is modern, direct, slightly playful. Keep that tone.
- **Facts > opinions.** If you don't have exact numbers, mark as `[NEEDS SPECIFICS]`.
- **Prioritize.** Hero > Headlines > CTA > body text > footer.
- **No over-drying.** Infostyle != "delete all adjectives". Replace empty words with meaningful ones.
- **Sales-led model.** All CTAs should lead to demo/sales contact, never free trials.
- **Product name.** Always "Keepstar One" (not just "Keepstar").
