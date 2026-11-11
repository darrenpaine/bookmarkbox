// Tag autocomplete for the Tags fields. A field with data-tag-suggest becomes
// a combobox (the ARIA pattern) listing the User's Tags that match the word
// being typed: "list" fields take Tags separated by commas, and "one" fields
// add a single Tag as soon as one is chosen. The Tags come from the page's
// <datalist id='tag-suggestions'>, most used first.
//
// Events are handled on the document, so fields that htmx swaps in work too.
// Without this script, the fields are plain text boxes.
(function () {
    "use strict";

    const wide = 8;   // suggestions shown at most
    const narrow = 6; // on small screens
    const small = window.matchMedia("(max-width: 719px)");

    let tags = null; // [{name, colour, count}], read once
    let field = null; // the field the list is open for
    let list = null; // its listbox
    let active = -1; // the highlighted option, or -1
    let ids = 0;

    function allTags() {
        if (tags === null) {
            const source = document.getElementById("tag-suggestions");
            tags = source ? Array.from(source.options, (o) => ({
                name: o.value,
                colour: o.dataset.colour,
                count: Number(o.dataset.count),
            })) : [];
        }
        return tags;
    }

    // setUp makes a field a combobox the first time it gets focus.
    function setUp(input) {
        if (input.getAttribute("role") === "combobox") {
            return;
        }
        if (!input.id) {
            input.id = "tag-field-" + ++ids;
        }
        const box = document.createElement("ul");
        box.id = input.id + "-suggestions";
        box.className = "suggestions";
        box.setAttribute("role", "listbox");
        box.setAttribute("aria-label", "Tag suggestions");
        box.hidden = true;
        input.after(box);
        input.setAttribute("role", "combobox");
        input.setAttribute("aria-autocomplete", "list");
        input.setAttribute("aria-expanded", "false");
        input.setAttribute("aria-controls", box.id);
    }

    // parts splits a field into the Tags already chosen and the word being
    // typed. A "one" field has only the word.
    function parts(input) {
        const value = input.value;
        if (input.dataset.tagSuggest !== "list") {
            return { before: "", chosen: [], word: value };
        }
        const cut = value.lastIndexOf(",") + 1;
        const before = value.slice(0, cut);
        return {
            before: before,
            chosen: before.split(",").map(clean).filter(Boolean),
            word: value.slice(cut),
        };
    }

    function clean(name) {
        return name.trim().toLowerCase();
    }

    // taken lists the Tags not worth suggesting: those already in the field,
    // and those already on the Bookmark in a Tag editor.
    function taken(input, chosen) {
        const names = new Set(chosen);
        const editor = input.closest(".tag-editor");
        if (editor) {
            editor.querySelectorAll("input[name='name'][type='hidden']").forEach((i) => names.add(i.value));
        }
        return names;
    }

    // matches gives the Tags for a word: those starting with it, then those
    // with a later word starting with it, then those containing it. Each
    // group keeps the most used first.
    function matches(word, skip) {
        const starts = [];
        const words = [];
        const inside = [];
        for (const tag of allTags()) {
            if (skip.has(tag.name)) {
                continue;
            }
            if (tag.name.startsWith(word)) {
                starts.push(tag);
            } else if (tag.name.split(/[\s\-_./]+/).slice(1).some((w) => w.startsWith(word))) {
                words.push(tag);
            } else if (tag.name.includes(word)) {
                inside.push(tag);
            }
        }
        return starts.concat(words, inside).slice(0, small.matches ? narrow : wide);
    }

    function update(input) {
        const p = parts(input);
        const word = clean(p.word);
        // Only suggest for the end of the field, where the typing is.
        if (word === "" || input.selectionStart !== input.value.length) {
            close();
            return;
        }
        const found = matches(word, taken(input, p.chosen));
        if (found.length === 0) {
            close();
            return;
        }
        open(input, found);
    }

    function open(input, found) {
        if (field !== input) {
            close();
        }
        field = input;
        list = document.getElementById(input.getAttribute("aria-controls"));
        list.replaceChildren(...found.map((tag, i) => {
            const option = document.createElement("li");
            option.id = list.id + "-" + i;
            option.setAttribute("role", "option");
            option.setAttribute("aria-selected", "false");
            option.dataset.name = tag.name;
            const chip = document.createElement("span");
            chip.className = "tag " + tag.colour;
            chip.textContent = tag.name;
            const count = document.createElement("span");
            count.className = "count";
            count.textContent = tag.count;
            count.setAttribute("aria-label", tag.count === 1 ? "1 bookmark" : tag.count + " bookmarks");
            option.append(chip, count);
            return option;
        }));
        list.hidden = false;
        input.setAttribute("aria-expanded", "true");
        highlight(-1);
    }

    function close() {
        if (field) {
            field.setAttribute("aria-expanded", "false");
            field.removeAttribute("aria-activedescendant");
        }
        if (list) {
            list.hidden = true;
            list.replaceChildren();
        }
        field = null;
        list = null;
        active = -1;
    }

    function highlight(i) {
        const options = list.children;
        active = i;
        Array.from(options).forEach((o, n) => o.setAttribute("aria-selected", String(n === i)));
        if (i >= 0) {
            field.setAttribute("aria-activedescendant", options[i].id);
            options[i].scrollIntoView({ block: "nearest" });
        } else {
            field.removeAttribute("aria-activedescendant");
        }
    }

    // accept puts a Tag in the field. A "list" field gets ", " after it, ready
    // for the next; a "one" field adds the Tag straight away.
    function accept(name) {
        const input = field;
        const p = parts(input);
        close();
        if (input.dataset.tagSuggest === "list") {
            const before = p.before === "" ? "" : p.before.trimEnd() + " ";
            input.value = before + name + ", ";
            input.setSelectionRange(input.value.length, input.value.length);
            input.focus();
        } else {
            input.value = name;
            input.form.requestSubmit();
        }
    }

    document.addEventListener("focusin", (e) => {
        if (e.target.matches("input[data-tag-suggest]")) {
            setUp(e.target);
        }
    });

    document.addEventListener("input", (e) => {
        if (e.target.matches("input[data-tag-suggest]")) {
            setUp(e.target);
            update(e.target);
        }
    });

    document.addEventListener("focusout", (e) => {
        if (e.target === field) {
            close();
        }
    });

    document.addEventListener("keydown", (e) => {
        if (e.target !== field || list === null) {
            return;
        }
        const count = list.children.length;
        switch (e.key) {
        case "ArrowDown":
            e.preventDefault();
            highlight(active + 1 < count ? active + 1 : 0);
            break;
        case "ArrowUp":
            e.preventDefault();
            highlight(active > 0 ? active - 1 : count - 1);
            break;
        case "Enter":
        case "Tab":
            if (active >= 0) {
                e.preventDefault();
                accept(list.children[active].dataset.name);
            } else {
                close();
            }
            break;
        case "Escape":
            e.preventDefault();
            close();
            break;
        }
    });

    // Keep the focus in the field while a suggestion is pressed, so the list
    // doesn't close before the click arrives.
    document.addEventListener("mousedown", (e) => {
        if (list && list.contains(e.target)) {
            e.preventDefault();
        }
    });

    document.addEventListener("click", (e) => {
        const option = list && e.target.closest(".suggestions [role='option']");
        if (option && list.contains(option)) {
            accept(option.dataset.name);
        }
    });

    // Follow the mouse only when it moves, not when the list moves under a
    // still pointer, which would steal the highlight from the keyboard.
    document.addEventListener("mousemove", (e) => {
        const option = list && e.target.closest(".suggestions [role='option']");
        if (option && list.contains(option) && option.getAttribute("aria-selected") !== "true") {
            highlight(Array.prototype.indexOf.call(list.children, option));
        }
    });
})();
