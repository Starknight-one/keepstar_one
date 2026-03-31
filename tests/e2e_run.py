"""E2E Test Runner — sends chat queries, validates visual assertions, takes screenshots.

Each test case has explicit expectations: layout, widget count, columns, fields.
A test FAILS if any assertion is violated — not just "widgets > 0".

After running, generates:
  - tests/e2e_results.json  — structured data for each test
  - tests/e2e_report.md     — markdown report with all details + screenshot refs
  - tests/e2e_screenshots/  — full viewport + widget-area crops per test
"""
from playwright.sync_api import sync_playwright
import time
import json
import os
from datetime import datetime

PROD_URL = "https://keepstar.one"
SCREENSHOT_DIR = "tests/e2e_screenshots"
RESULTS_FILE = "tests/e2e_results.json"
REPORT_FILE = "tests/e2e_report.md"
MAX_WAIT = 30  # max seconds to wait for response

# ─── Test Cases with Assertions ─────────────────────────────────────────────

TEST_CASES = [
    # Block 1: Basic search + auto-resolve
    {
        "id": 1,
        "query": "Привет, покажи кремы для лица",
        "block": "1_basic",
        "expect": {
            "layout": "grid",
            "columns": 3,
            "widgets_min": 10,
            "widgets_max": 30,
            "size": "small",
            "fields_present": ["hero", "title", "price"],
            "fields_absent": [],
        },
    },
    {
        "id": 2,
        "query": "Покажи их списком",
        "block": "1_basic",
        "expect": {
            "layout": "list",
            "widgets_min": 10,
            "widgets_max": 30,
            "fields_present": ["hero", "title", "price"],
            "fields_absent": [],
        },
    },
    {
        "id": 3,
        "query": "Покажи детально первый товар",
        "block": "1_basic",
        "expect": {
            "layout": "single",
            "widgets_min": 1,
            "widgets_max": 1,
            "size": "large",
            "fields_present": ["hero", "title", "price"],
            "fields_absent": [],
        },
    },
    # Block 2: Show/Hide — editing detail card (still single after #3)
    {
        "id": 4,
        "query": "Покажи только название и цены",
        "block": "2_show_hide",
        "expect": {
            "layout": "single",
            "widgets_min": 1,
            "widgets_max": 1,
            "fields_present": ["title", "price"],
            "fields_absent": ["hero"],
        },
    },
    {
        "id": 5,
        "query": "Добавь рейтинг",
        "block": "2_show_hide",
        "expect": {
            "layout": "single",
            "widgets_min": 1,
            "widgets_max": 1,
            "fields_present": ["title", "price"],
            "fields_absent": ["hero"],
            "has_rating": True,
        },
    },
    {
        "id": 6,
        "query": "Убери цены",
        "block": "2_show_hide",
        "expect": {
            "layout": "single",
            "widgets_min": 1,
            "widgets_max": 1,
            "fields_present": ["title"],
            "fields_absent": ["hero", "price"],
            "has_rating": True,
        },
    },
    {
        "id": 7,
        "query": "Покажи всё как было в самом начале",
        "block": "2_show_hide",
        "expect": {
            "layout": "grid",
            "columns": 3,
            "widgets_min": 10,
            "widgets_max": 30,
            "size": "small",
            "fields_present": ["hero", "title", "price"],
            "fields_absent": [],
        },
    },
    # Block 3: Order (preset persistence)
    {
        "id": 8,
        "query": "Покажи цену первой, потом название",
        "block": "3_order",
        "expect": {
            "layout": "grid",
            "widgets_min": 10,
            "widgets_max": 30,
            "fields_present": ["title", "price"],
            "price_before_title": True,
        },
    },
    # Block 4: Size / Layout switching
    {
        "id": 9,
        "query": "Покажи крупными карточками",
        "block": "4_size_layout",
        "expect": {
            "layout": "grid",
            "columns_max": 2,
            "widgets_min": 10,
            "widgets_max": 30,
            "size": "large",
            "fields_present": ["hero", "title", "price"],
        },
    },
    {
        "id": 10,
        "query": "Покажи каруселью",
        "block": "4_size_layout",
        "expect": {
            "layout": "carousel",
            "widgets_min": 10,
            "widgets_max": 30,
            "card_fits_viewport": True,
        },
    },
    {
        "id": 11,
        "query": "Покажи таблицей",
        "block": "4_size_layout",
        "expect": {
            "layout": "table",
            "widgets_min": 10,
            "widgets_max": 30,
        },
    },
    {
        "id": 12,
        "query": "Покажи снова сеткой, маленькими",
        "block": "4_size_layout",
        "expect": {
            "layout": "grid",
            "columns_min": 4,
            "columns_max": 5,
            "widgets_min": 10,
            "widgets_max": 30,
            "size": "small",
        },
    },
    {
        "id": 13,
        "query": "Покажи горизонтальными карточками",
        "block": "4_size_layout",
        "expect": {
            "layout_any": ["list", "grid"],
            "widgets_min": 10,
            "widgets_max": 30,
            "direction_horizontal": True,
        },
    },
    # Block 5: Filtering + brand
    {
        "id": 14,
        "query": "Покажи только COSRX",
        "block": "5_filter",
        "expect": {
            "layout": "grid",
            "widgets_min": 2,
            "widgets_max": 8,
            "content_contains": "COSRX",
        },
    },
    {
        "id": 15,
        "query": "Покажи их с составом и типом кожи",
        "block": "5_filter",
        "expect": {
            "layout": "grid",
            "widgets_min": 2,
            "widgets_max": 8,
            "content_contains": "COSRX",
        },
    },
    {
        "id": 16,
        "query": "Покажи все кремы The Ordinary",
        "block": "5_filter",
        "expect": {
            "layout": "grid",
            "widgets_min": 1,
            "widgets_max": 30,
            "content_not_only": "COSRX",
        },
    },
    # Block 6: Complex combinations
    {
        "id": 17,
        "query": "Покажи топ-5 с рейтингом, брендом и описанием, крупно",
        "block": "6_complex",
        "expect": {
            "layout": "grid",
            "widgets_min": 4,
            "widgets_max": 6,
            "size": "large",
        },
    },
    {
        "id": 18,
        "query": "Сравни первые 3",
        "block": "6_complex",
        "expect": {
            "layout_any": ["comparison", "table"],
            "widgets_min": 2,
            "widgets_max": 4,
        },
    },
    {
        "id": 19,
        "query": "Сравни два самых дешёвых",
        "block": "6_complex",
        "expect": {
            "layout_any": ["comparison", "table"],
            "widgets_min": 2,
            "widgets_max": 3,
        },
    },
    # Block 7: New search + drill-down
    {
        "id": 20,
        "query": "Покажи все увлажняющие средства",
        "block": "7_drilldown",
        "expect": {
            "layout": "grid",
            "widgets_min": 5,
            "widgets_max": 50,
        },
    },
    {
        "id": 21,
        "query": "Покажи первые 3 крупными карточками с описанием",
        "block": "7_drilldown",
        "expect": {
            "layout": "grid",
            "widgets_min": 2,
            "widgets_max": 4,
            "size": "large",
        },
    },
    {
        "id": 22,
        "query": "Покажи детально вторую",
        "block": "7_drilldown",
        "expect": {
            "layout": "single",
            "widgets_min": 1,
            "widgets_max": 1,
        },
    },
    # Block 8: V2-specific
    {
        "id": 23,
        "query": "Покажи сеткой по 2 в ряд",
        "block": "8_v2",
        "expect": {
            "layout": "grid",
            "columns": 2,
            "widgets_min": 5,
            "widgets_max": 50,
        },
    },
    {
        "id": 24,
        "query": "Покажи с тенями и акцентным цветом",
        "block": "8_v2",
        "expect": {
            "layout": "grid",
            "widgets_min": 5,
            "widgets_max": 50,
            "has_shadow": True,
        },
    },
    {
        "id": 25,
        "query": "Покажи топ-3 сыворотки",
        "block": "8_v2",
        "expect": {
            "layout": "grid",
            "widgets_min": 2,
            "widgets_max": 4,
        },
    },
]


# ─── Shadow DOM helpers ─────────────────────────────────────────────────────

SR_JS = """
    let sr = null;
    document.querySelectorAll('*').forEach(el => { if (el.shadowRoot) sr = el.shadowRoot; });
"""


def send_message(page, text):
    """Type and send a message inside shadow DOM."""
    page.evaluate(f"""(text) => {{
        {SR_JS}
        if (!sr) return;
        const textarea = sr.querySelector('textarea');
        if (!textarea) return;
        const setter = Object.getOwnPropertyDescriptor(
            window.HTMLTextAreaElement.prototype, 'value'
        ).set;
        setter.call(textarea, text);
        textarea.dispatchEvent(new Event('input', {{ bubbles: true }}));
        const btns = sr.querySelectorAll('button[type="submit"]');
        const sendBtn = btns[btns.length - 1];
        if (sendBtn) sendBtn.click();
    }}""", text)


def get_message_count(page):
    return page.evaluate(f"""() => {{
        {SR_JS}
        if (!sr) return 0;
        return sr.querySelectorAll('.message-bubble, .message').length;
    }}""")


def has_loading(page):
    return page.evaluate(f"""() => {{
        {SR_JS}
        if (!sr) return false;
        const indicators = sr.querySelectorAll('.typing-indicator, .spinner, .chat-input-sending');
        for (const el of indicators) {{
            const style = window.getComputedStyle(el);
            if (style.display !== 'none' && style.visibility !== 'hidden') return true;
        }}
        return false;
    }}""")


def wait_for_response(page, prev_msg_count):
    """Wait until new message appears and loading stops."""
    start = time.time()

    # Phase 1: wait for message count to increase
    while time.time() - start < MAX_WAIT:
        if get_message_count(page) > prev_msg_count:
            break
        time.sleep(0.3)

    # Phase 2: wait for loading to stop
    while time.time() - start < MAX_WAIT:
        if not has_loading(page):
            time.sleep(1.0)
            if not has_loading(page):
                break
        time.sleep(0.3)

    # Phase 3: wait for widgets to render
    widget_wait_start = time.time()
    while time.time() - start < MAX_WAIT:
        wc = page.evaluate(f"""() => {{
            {SR_JS}
            if (!sr) return 0;
            return sr.querySelectorAll('.generic-card, .comparison-column, .formation-table').length;
        }}""")
        if wc > 0:
            time.sleep(1.5)
            break
        if time.time() - widget_wait_start > 5:
            break
        time.sleep(0.3)

    return round(time.time() - start, 1)


# ─── Deep inspection of current visual state ────────────────────────────────

def inspect_state(page):
    """Extract detailed visual state from shadow DOM for assertion checking."""
    return page.evaluate(f"""() => {{
        {SR_JS}
        if (!sr) return null;

        const result = {{
            layout: null,
            columns: null,
            widgets: 0,
            size: null,
            fields: [],
            slots: [],
            has_rating: false,
            has_shadow: false,
            direction_horizontal: false,
            card_fits_viewport: true,
            price_before_title: false,
            text_content: '',
        }};

        // 1. Detect layout from formation class
        const formationEl = sr.querySelector(
            '.formation-grid, .formation-list, .formation-carousel, ' +
            '.formation-single, .formation-comparison, .formation-table, .comparison-table'
        );
        if (formationEl) {{
            const cls = formationEl.className;
            if (cls.includes('formation-grid'))       result.layout = 'grid';
            else if (cls.includes('formation-list'))   result.layout = 'list';
            else if (cls.includes('formation-carousel')) result.layout = 'carousel';
            else if (cls.includes('formation-single')) result.layout = 'single';
            else if (cls.includes('formation-comparison') || cls.includes('comparison-table'))
                result.layout = 'comparison';
            else if (cls.includes('formation-table'))  result.layout = 'table';

            // columns from cols-N class
            const colsMatch = cls.match(/cols-(\\d+)/);
            if (colsMatch) result.columns = parseInt(colsMatch[1]);
        }}

        // 2. Count widgets
        const cards = sr.querySelectorAll('.generic-card, .comparison-column');
        result.widgets = cards.length;

        // 3. Detect size from first card
        if (cards.length > 0) {{
            const cls = cards[0].className;
            if (cls.includes('size-large'))       result.size = 'large';
            else if (cls.includes('size-medium'))  result.size = 'medium';
            else if (cls.includes('size-small'))   result.size = 'small';
            else if (cls.includes('size-tiny'))    result.size = 'tiny';
        }}

        // 4. Collect slots (data-slot attributes) from first card
        if (cards.length > 0) {{
            const atoms = cards[0].querySelectorAll('[data-slot]');
            const slotSet = new Set();
            atoms.forEach(a => slotSet.add(a.getAttribute('data-slot')));
            result.slots = Array.from(slotSet);

            // Check for rating (display-rating class or data-slot="rating")
            const ratingEl = cards[0].querySelector('.display-rating, [data-slot="rating"], .atom-v2-text.rating');
            result.has_rating = !!ratingEl;
        }}

        // 5. Collect all unique slots across all cards
        const allSlots = new Set();
        cards.forEach(card => {{
            card.querySelectorAll('[data-slot]').forEach(a => {{
                allSlots.add(a.getAttribute('data-slot'));
            }});
        }});
        result.fields = Array.from(allSlots);

        // 6. Check if price appears before title in DOM order (first card)
        if (cards.length > 0) {{
            const allAtoms = Array.from(cards[0].querySelectorAll('[data-slot]'));
            let priceIdx = -1, titleIdx = -1;
            allAtoms.forEach((a, i) => {{
                const slot = a.getAttribute('data-slot');
                if (slot === 'price' && priceIdx === -1) priceIdx = i;
                if (slot === 'title' && titleIdx === -1) titleIdx = i;
            }});
            if (priceIdx >= 0 && titleIdx >= 0) {{
                result.price_before_title = priceIdx < titleIdx;
            }}
        }}

        // 7. Check box-shadow on cards
        if (cards.length > 0) {{
            const style = window.getComputedStyle(cards[0]);
            const shadow = style.boxShadow;
            result.has_shadow = shadow && shadow !== 'none' && shadow !== '';
        }}

        // 8. Check horizontal direction — card media and content side by side
        if (cards.length > 0) {{
            const card = cards[0];
            const style = window.getComputedStyle(card);
            // Check if card itself is flex row
            if (style.flexDirection === 'row') {{
                result.direction_horizontal = true;
            }}
            // Also check for layout-row class
            if (card.querySelector('.layout-row') || card.classList.contains('horizontal')) {{
                result.direction_horizontal = true;
            }}
        }}

        // 9. Card fits viewport check (for carousel)
        const displayArea = sr.querySelector('.widget-display-area');
        if (displayArea && cards.length > 0) {{
            const areaRect = displayArea.getBoundingClientRect();
            const cardRect = cards[0].getBoundingClientRect();
            result.card_fits_viewport = cardRect.bottom <= areaRect.bottom + 10;
        }}

        // 10. Text content (for content_contains checks)
        const displayAreaEl = sr.querySelector('.widget-display-area');
        if (displayAreaEl) {{
            result.text_content = displayAreaEl.textContent.substring(0, 5000);
        }}

        // 11. Card titles (first 5 cards)
        result.card_titles = [];
        const titleCards = Array.from(cards).slice(0, 5);
        titleCards.forEach(card => {{
            const titleEl = card.querySelector('[data-slot="title"]');
            if (titleEl) result.card_titles.push(titleEl.textContent.trim().substring(0, 80));
        }});

        // 12. Last bot message text
        const messages = sr.querySelectorAll('.message-bubble, .message');
        if (messages.length > 0) {{
            const lastMsg = messages[messages.length - 1];
            result.bot_message = lastMsg.textContent.trim().substring(0, 300);
        }}

        // 13. Formation wrapper class (full, for debugging)
        const fw = sr.querySelector('.formation-wrapper');
        if (fw) result.formation_class = fw.innerHTML.substring(0, 200);

        // 14. First card computed styles
        if (cards.length > 0) {{
            const cs = window.getComputedStyle(cards[0]);
            result.card_css = {{
                borderRadius: cs.borderRadius,
                boxShadow: cs.boxShadow === 'none' ? null : cs.boxShadow,
                flexDirection: cs.flexDirection,
                width: cs.width,
                height: cs.height,
            }};
        }}

        return result;
    }}""")


# ─── Assertion engine ────────────────────────────────────────────────────────

def check_assertions(expect, state):
    """Compare expectations against actual state. Returns list of failures."""
    if state is None:
        return ["CRITICAL: could not inspect shadow DOM state"]

    failures = []

    # Layout
    if "layout" in expect:
        if state["layout"] != expect["layout"]:
            failures.append(
                f"LAYOUT: expected '{expect['layout']}', got '{state['layout']}'"
            )
    if "layout_any" in expect:
        if state["layout"] not in expect["layout_any"]:
            failures.append(
                f"LAYOUT: expected one of {expect['layout_any']}, got '{state['layout']}'"
            )

    # Widget count
    wc = state["widgets"]
    if "widgets_min" in expect and wc < expect["widgets_min"]:
        failures.append(
            f"WIDGETS: expected >= {expect['widgets_min']}, got {wc}"
        )
    if "widgets_max" in expect and wc > expect["widgets_max"]:
        failures.append(
            f"WIDGETS: expected <= {expect['widgets_max']}, got {wc}"
        )

    # Columns (exact)
    if "columns" in expect:
        if state["columns"] != expect["columns"]:
            failures.append(
                f"COLUMNS: expected {expect['columns']}, got {state['columns']}"
            )
    if "columns_min" in expect and (state["columns"] is None or state["columns"] < expect["columns_min"]):
        failures.append(
            f"COLUMNS: expected >= {expect['columns_min']}, got {state['columns']}"
        )
    if "columns_max" in expect and state["columns"] is not None and state["columns"] > expect["columns_max"]:
        failures.append(
            f"COLUMNS: expected <= {expect['columns_max']}, got {state['columns']}"
        )

    # Size
    if "size" in expect:
        if state["size"] != expect["size"]:
            failures.append(
                f"SIZE: expected '{expect['size']}', got '{state['size']}'"
            )

    # Fields present (check slots on first card)
    slots = set(state.get("slots", []))
    for field in expect.get("fields_present", []):
        if field not in slots:
            failures.append(
                f"FIELD MISSING: expected slot '{field}' in card, found: {sorted(slots)}"
            )

    # Fields absent
    for field in expect.get("fields_absent", []):
        if field in slots:
            failures.append(
                f"FIELD UNWANTED: slot '{field}' should NOT be in card, found: {sorted(slots)}"
            )

    # Rating
    if expect.get("has_rating") and not state.get("has_rating"):
        failures.append("RATING: expected rating to be visible, not found")

    # Price before title (order check)
    if expect.get("price_before_title") and not state.get("price_before_title"):
        failures.append("ORDER: expected price BEFORE title in DOM, but title comes first")

    # Shadow
    if expect.get("has_shadow") and not state.get("has_shadow"):
        failures.append("SHADOW: expected box-shadow on cards, not found")

    # Direction horizontal
    if expect.get("direction_horizontal") and not state.get("direction_horizontal"):
        failures.append("DIRECTION: expected horizontal card layout, not detected")

    # Card fits viewport
    if expect.get("card_fits_viewport") and not state.get("card_fits_viewport"):
        failures.append("VIEWPORT: card overflows display area")

    # Content contains text
    if "content_contains" in expect:
        text = state.get("text_content", "")
        if expect["content_contains"].lower() not in text.lower():
            failures.append(
                f"CONTENT: expected text '{expect['content_contains']}' in widget area"
            )

    # Content NOT only from one brand (check that new search happened)
    if "content_not_only" in expect:
        text = state.get("text_content", "")
        brand = expect["content_not_only"]
        # Crude check: if brand appears in every card name, probably didn't switch
        # This is a soft check — we just flag if the text is dominated by the brand
        brand_count = text.lower().count(brand.lower())
        if brand_count > 5 and wc > 3:
            failures.append(
                f"CONTENT: appears to still show '{brand}' products ({brand_count} mentions), expected new search"
            )

    return failures


# ─── Main runner ─────────────────────────────────────────────────────────────

def run_tests():
    results = []

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 900})
        page.goto(PROD_URL)
        page.wait_for_load_state("networkidle")
        page.wait_for_timeout(3000)

        # Open chat widget
        page.evaluate(f"""() => {{
            {SR_JS}
            if (sr) {{
                const btn = sr.querySelector('.chat-toggle-circle');
                if (btn) btn.click();
            }}
        }}""")
        page.wait_for_timeout(2000)

        total_pass = 0
        total_fail = 0

        WAIT_AFTER_SEND = 5  # fixed wait (seconds)

        for tc in TEST_CASES:
            test_id = tc["id"]
            query = tc["query"]
            block = tc["block"]
            expect = tc["expect"]

            print(f"\n{'='*70}")
            print(f"#{test_id:02d} | {query}")
            print(f"     Expect: layout={expect.get('layout', expect.get('layout_any', '?'))}, "
                  f"widgets={expect.get('widgets_min', '?')}-{expect.get('widgets_max', '?')}")

            send_message(page, query)
            time.sleep(WAIT_AFTER_SEND)
            elapsed = WAIT_AFTER_SEND

            # Deep inspection
            state = inspect_state(page)

            # Screenshot
            screenshot_path = f"{SCREENSHOT_DIR}/{test_id:02d}_{block}.png"
            page.screenshot(path=screenshot_path)

            # Run assertions
            failures = check_assertions(expect, state)

            if state:
                print(f"     Got:    layout={state['layout']}, widgets={state['widgets']}, "
                      f"cols={state['columns']}, size={state['size']}, "
                      f"slots={state.get('slots', [])}")

            if failures:
                status = "FAIL"
                total_fail += 1
                for f in failures:
                    print(f"     ✗ {f}")
            else:
                status = "PASS"
                total_pass += 1
                print(f"     ✓ All assertions passed")

            print(f"     [{elapsed}s]")

            result = {
                "id": test_id,
                "query": query,
                "block": block,
                "status": status,
                "time_sec": elapsed,
                "screenshot": screenshot_path,
                "actual": {
                    "layout": state["layout"] if state else None,
                    "widgets": state["widgets"] if state else 0,
                    "columns": state["columns"] if state else None,
                    "size": state["size"] if state else None,
                    "slots": state.get("slots", []) if state else [],
                    "fields": state.get("fields", []) if state else [],
                    "has_rating": state.get("has_rating") if state else False,
                    "has_shadow": state.get("has_shadow") if state else False,
                    "price_before_title": state.get("price_before_title") if state else False,
                    "direction_horizontal": state.get("direction_horizontal") if state else False,
                    "card_titles": state.get("card_titles", []) if state else [],
                    "bot_message": state.get("bot_message", "") if state else "",
                    "card_css": state.get("card_css") if state else None,
                },
                "failures": failures,
                "expected": expect,
            }
            results.append(result)

        browser.close()

    # Save results
    with open(RESULTS_FILE, 'w') as f:
        json.dump(results, f, indent=2, ensure_ascii=False)

    # Summary
    print("\n" + "=" * 80)
    print(f"{'#':>3} | {'Status':<6} | {'Layout':<12} | {'W':>3} | {'Cols':>4} | {'Fails':>5} | Query")
    print("-" * 80)
    for r in results:
        a = r["actual"]
        fail_count = len(r["failures"])
        print(f"{r['id']:>3} | {r['status']:<6} | {str(a['layout']):<12} | "
              f"{a['widgets']:>3} | {str(a['columns']):>4} | {fail_count:>5} | {r['query'][:35]}")

    print(f"\n{'='*80}")
    print(f"PASS: {total_pass}  |  FAIL: {total_fail}  |  TOTAL: {total_pass + total_fail}")

    if total_fail > 0:
        print(f"\nFailed tests:")
        for r in results:
            if r["status"] == "FAIL":
                print(f"  #{r['id']:02d}: {', '.join(r['failures'])}")

    times = [r['time_sec'] for r in results]
    if times:
        print(f"\nAvg response time: {sum(times)/len(times):.1f}s")

    print(f"\nScreenshots: {SCREENSHOT_DIR}/")
    print(f"Results: {RESULTS_FILE}")

    # Generate markdown report
    generate_report(results)
    print(f"Report: {REPORT_FILE}")


def generate_report(results):
    """Generate a detailed markdown report for visual review."""
    now = datetime.now().strftime("%Y-%m-%d %H:%M")
    total = len(results)
    passed = sum(1 for r in results if r["status"] == "PASS")
    failed = total - passed
    times = [r["time_sec"] for r in results]
    avg_time = sum(times) / len(times) if times else 0

    lines = [
        f"# E2E Test Report — {now}",
        f"",
        f"**URL**: {PROD_URL}",
        f"**Tests**: {total} | **PASS**: {passed} | **FAIL**: {failed}",
        f"**Avg response**: {avg_time:.1f}s | **Min**: {min(times):.1f}s | **Max**: {max(times):.1f}s",
        f"",
        f"## Summary",
        f"",
        f"| # | Status | Time | Layout | Widgets | Cols | Size | Query |",
        f"|---|--------|------|--------|---------|------|------|-------|",
    ]

    for r in results:
        a = r["actual"]
        status_icon = "PASS" if r["status"] == "PASS" else "**FAIL**"
        lines.append(
            f"| {r['id']} | {status_icon} | {r['time_sec']}s | "
            f"{a['layout'] or '—'} | {a['widgets']} | {a['columns'] or '—'} | "
            f"{a['size'] or '—'} | {r['query'][:40]} |"
        )

    lines += ["", "---", ""]

    # Detailed per-test sections
    current_block = None
    for r in results:
        if r["block"] != current_block:
            current_block = r["block"]
            lines += [f"## Block: {current_block}", ""]

        a = r["actual"]
        e = r["expected"]
        status_marker = "PASS" if r["status"] == "PASS" else "FAIL"

        lines += [
            f"### #{r['id']:02d} — {status_marker}",
            f"",
            f"**Query**: {r['query']}",
            f"**Time**: {r['time_sec']}s",
            f"",
            f"| | Expected | Actual |",
            f"|---|----------|--------|",
            f"| Layout | {e.get('layout', e.get('layout_any', '—'))} | {a['layout'] or '—'} |",
            f"| Widgets | {e.get('widgets_min', '?')}–{e.get('widgets_max', '?')} | {a['widgets']} |",
            f"| Columns | {e.get('columns', e.get('columns_min', '—'))}{'–'+str(e['columns_max']) if 'columns_max' in e else ''} | {a['columns'] or '—'} |",
            f"| Size | {e.get('size', '—')} | {a['size'] or '—'} |",
            f"| Slots (1st card) | {', '.join(e.get('fields_present', []))} | {', '.join(a.get('slots', []))} |",
        ]

        if a.get("card_css"):
            css = a["card_css"]
            lines.append(f"| Card CSS | — | border-radius: {css.get('borderRadius', '—')}, shadow: {'yes' if css.get('boxShadow') else 'no'}, flex: {css.get('flexDirection', '—')}, w: {css.get('width', '—')} |")

        if a.get("bot_message"):
            msg = a["bot_message"][:150].replace("|", "\\|").replace("\n", " ")
            lines.append(f"")
            lines.append(f"**Bot said**: {msg}")

        if a.get("card_titles"):
            titles = ", ".join(a["card_titles"][:3])
            lines.append(f"**Cards**: {titles}")

        if r["failures"]:
            lines.append(f"")
            lines.append(f"**Failures**:")
            for f in r["failures"]:
                lines.append(f"- {f}")

        lines += [
            f"",
            f"**Screenshot**: `{r['screenshot']}`",
            f"",
            f"> **Visual review**: _TODO — check screenshot_",
            f"",
            f"---",
            f"",
        ]

    with open(REPORT_FILE, "w") as f:
        f.write("\n".join(lines))


if __name__ == "__main__":
    run_tests()
