"""Reconnaissance: discover widget DOM after opening chat."""
from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 430, "height": 932})
    page.goto("http://localhost:5173")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(2000)

    # Find shadow host and click toggle
    page.evaluate("""() => {
        const hosts = document.querySelectorAll('*');
        for (const h of hosts) {
            if (h.shadowRoot) {
                const btn = h.shadowRoot.querySelector('.chat-toggle-circle');
                if (btn) btn.click();
                return;
            }
        }
    }""")
    page.wait_for_timeout(1500)
    page.screenshot(path="tests/e2e_screenshots/00_recon_opened.png", full_page=True)

    # Now find inputs and buttons inside shadow DOM
    info = page.evaluate("""() => {
        const results = { inputs: [], buttons: [], structure: '' };
        document.querySelectorAll('*').forEach(host => {
            if (host.shadowRoot) {
                host.shadowRoot.querySelectorAll('input, textarea').forEach(el => {
                    results.inputs.push({ tag: el.tagName, type: el.type, placeholder: el.placeholder, id: el.id, class: el.className });
                });
                host.shadowRoot.querySelectorAll('button, [role="button"]').forEach(el => {
                    results.buttons.push({ text: el.textContent.trim().substring(0, 50), class: el.className, id: el.id, type: el.type || '' });
                });
                // Get chat panel structure
                const panel = host.shadowRoot.querySelector('.chat-panel, [class*="chat"]');
                if (panel) results.structure = panel.innerHTML.substring(0, 2000);
            }
        });
        return results;
    }""")

    print("=== Inputs ===")
    for i in info['inputs']:
        print(i)
    print("\n=== Buttons ===")
    for b in info['buttons']:
        print(b)
    print("\n=== Chat structure ===")
    print(info['structure'][:1500])

    browser.close()
