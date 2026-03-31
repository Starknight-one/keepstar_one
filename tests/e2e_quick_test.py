"""Quick test: find the right clip area for widget overlay."""
from playwright.sync_api import sync_playwright
import time

PROD_URL = "https://keepstar.one"

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 900})
    page.goto(PROD_URL)
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(3000)

    # Open chat
    page.evaluate("""() => {
        document.querySelectorAll('*').forEach(h => {
            if (h.shadowRoot) {
                const btn = h.shadowRoot.querySelector('.chat-toggle-circle');
                if (btn) btn.click();
            }
        });
    }""")
    page.wait_for_timeout(2000)

    # Find overlay elements inside shadow DOM — they use position:fixed so getBoundingClientRect works
    boxes = page.evaluate("""() => {
        let sr = null;
        document.querySelectorAll('*').forEach(el => { if (el.shadowRoot) sr = el.shadowRoot; });
        if (!sr) return {};

        const result = {};
        const selectors = [
            '.chat-backdrop', '.widget-display-area', '.chat-area', '.chat-panel',
            '.chat-container', '.chat-wrapper', '[class*="chat-panel"]', '[class*="overlay"]'
        ];
        for (const sel of selectors) {
            const el = sr.querySelector(sel);
            if (el) {
                const r = el.getBoundingClientRect();
                result[sel] = { x: r.x, y: r.y, width: r.width, height: r.height };
            }
        }
        // Also get all direct children of shadow root with their classes
        const children = [];
        sr.querySelectorAll(':scope > *').forEach(el => {
            if (el.tagName !== 'STYLE' && el.tagName !== 'LINK') {
                const r = el.getBoundingClientRect();
                children.push({ tag: el.tagName, class: el.className, x: r.x, y: r.y, w: r.width, h: r.height });
            }
        });
        result['_children'] = children;
        return result;
    }""")

    print("=== Shadow DOM elements ===")
    for k, v in boxes.items():
        print(f"  {k}: {v}")

    # Try screenshot with viewport clip (the overlay covers the entire viewport)
    page.screenshot(path="tests/e2e_screenshots/quick_viewport.png")
    print("\nFull viewport screenshot saved")

    browser.close()
