(function() {
    'use strict';

    // State: 'inactive' | 'active' | 'paused'
    let state = 'inactive';
    let panel = null;
    let textarea = null;
    let currentHighlight = null;
    let originalHtml = null;

    // Fetch original HTML for line numbers
    fetch(window.location.href)
        .then(r => r.text())
        .then(html => originalHtml = html)
        .catch(() => originalHtml = null);

    // Create inspector panel
    function createPanel() {
        if (panel) return;

        panel = document.createElement('div');
        panel.id = 'dev-inspector-panel';
        panel.innerHTML = `
            <div class="inspector-header">
                <span>Inspector</span>
                <button id="inspector-copy-btn" title="Copy">Copy</button>
            </div>
            <textarea id="inspector-textarea" placeholder="Click an element to inspect..."></textarea>
            <div class="inspector-hint">Esc to close</div>
        `;
        document.body.appendChild(panel);

        textarea = document.getElementById('inspector-textarea');

        document.getElementById('inspector-copy-btn').addEventListener('click', () => {
            navigator.clipboard.writeText(textarea.value);
            const btn = document.getElementById('inspector-copy-btn');
            btn.textContent = 'Copied!';
            setTimeout(() => btn.textContent = 'Copy', 1000);
        });
    }

    function removePanel() {
        if (panel) {
            panel.remove();
            panel = null;
            textarea = null;
        }
    }

    function getLineNumber(element) {
        if (!originalHtml) return null;

        // Get a unique identifier from the element
        const outerHtml = element.outerHTML;
        const searchStr = outerHtml.substring(0, Math.min(100, outerHtml.indexOf('>') + 1));

        const lines = originalHtml.split('\n');
        for (let i = 0; i < lines.length; i++) {
            if (lines[i].includes(searchStr.trim().substring(0, 50))) {
                return i + 1;
            }
        }
        return null;
    }

    function getScreenPosition(element) {
        const rect = element.getBoundingClientRect();
        const viewportHeight = window.innerHeight;
        const viewportWidth = window.innerWidth;

        let vertical = '';
        let horizontal = '';

        if (rect.top < viewportHeight / 3) vertical = 'top';
        else if (rect.top < viewportHeight * 2 / 3) vertical = 'middle';
        else vertical = 'bottom';

        if (rect.left < viewportWidth / 3) horizontal = 'left';
        else if (rect.left < viewportWidth * 2 / 3) horizontal = 'center';
        else horizontal = 'right';

        return `${vertical} ${horizontal}`;
    }

    function formatElementInfo(element) {
        const outer = element.outerHTML;
        const lines = outer.split('\n').slice(0, 4);
        let code = lines.join('\n');

        // Truncate if too long
        if (code.length > 200) {
            code = code.substring(0, 200) + '...';
        }

        const lineNum = getLineNumber(element);
        const position = getScreenPosition(element);

        let result = code;
        if (lineNum) {
            result += `\n\n(line ${lineNum})`;
        }
        result += `\nScreen position: ${position}`;

        return result;
    }

    function highlightElement(element) {
        if (currentHighlight) {
            currentHighlight.classList.remove('inspector-highlight');
        }
        if (element && !panel.contains(element)) {
            element.classList.add('inspector-highlight');
            currentHighlight = element;
        }
    }

    function handleMouseOver(e) {
        if (state !== 'active') return;
        if (panel && panel.contains(e.target)) return;
        highlightElement(e.target);
    }

    function handleMouseOut(e) {
        if (state !== 'active') return;
        if (currentHighlight) {
            currentHighlight.classList.remove('inspector-highlight');
            currentHighlight = null;
        }
    }

    function handleClick(e) {
        if (state === 'inactive') return;
        if (panel && panel.contains(e.target)) return;

        e.preventDefault();
        e.stopPropagation();

        if (state === 'active') {
            // Select element and pause
            const info = formatElementInfo(e.target);
            if (textarea) {
                // Append to existing text
                if (textarea.value) {
                    textarea.value += '\n\n---\n\n' + info;
                } else {
                    textarea.value = info;
                }
                textarea.scrollTop = textarea.scrollHeight;
            }
            highlightElement(null);
            state = 'paused';
            panel.classList.add('paused');
        } else if (state === 'paused') {
            // Resume
            state = 'active';
            panel.classList.remove('paused');
        }
    }

    function handleKeyDown(e) {
        // Ctrl+Shift+K or Cmd+Shift+K (Mac) - toggle
        if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.code === 'KeyK') {
            e.preventDefault();

            if (state === 'inactive') {
                // Activate
                createPanel();
                state = 'active';
                document.body.classList.add('inspector-active');
            } else {
                // Deactivate
                highlightElement(null);
                removePanel();
                state = 'inactive';
                document.body.classList.remove('inspector-active');
            }
        }

        // Escape - close inspector
        if ((e.code === 'Escape' || e.key === 'Escape') && state !== 'inactive') {
            e.preventDefault();
            highlightElement(null);
            removePanel();
            state = 'inactive';
            document.body.classList.remove('inspector-active');
        }
    }

    // Event listeners
    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('mouseover', handleMouseOver, true);
    document.addEventListener('mouseout', handleMouseOut, true);
    document.addEventListener('click', handleClick, true);

    console.log('Dev Inspector loaded. Press Ctrl+Shift+K to activate.');
})();
