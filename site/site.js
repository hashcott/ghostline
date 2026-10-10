// Shared by every page: the header and footer, the language, and turning
// the repository's Markdown (copied next to the pages by the pages
// workflow) into page content.
window.Site = (() => {
  const REPO = "hashcott/ghostline";
  const GH = `https://github.com/${REPO}`;

  const STR = {
    vi: {
      pages: { index: "Tải về", guide: "Hướng dẫn", faq: "Hỏi đáp", changelog: "Thay đổi", disclaimer: "Miễn trừ trách nhiệm" },
      source: "Mã nguồn", issues: "Báo lỗi",
      license: "Ghostline là phần mềm mã nguồn mở theo giấy phép GPL-3.0.",
      lawful: 'Hãy dùng Ghostline đúng pháp luật nơi bạn sống. Xem <a href="disclaimer.html">miễn trừ trách nhiệm</a>.',
      loading: "Đang tải…",
      failed: (url) => `Không tải được nội dung. Xem trên <a href="${url}">GitHub</a>.`,
      callout: { NOTE: "Lưu ý", TIP: "Mẹo", IMPORTANT: "Quan trọng", WARNING: "Cảnh báo", CAUTION: "Thận trọng" },
    },
    en: {
      pages: { index: "Download", guide: "Guide", faq: "FAQ", changelog: "Changelog", disclaimer: "Disclaimer" },
      source: "Source code", issues: "Report a problem",
      license: "Ghostline is open-source software under the GPL-3.0 license.",
      lawful: 'Use Ghostline within the law where you live. See the <a href="disclaimer.html">disclaimer</a>.',
      loading: "Loading…",
      failed: (url) => `The content could not be loaded. Read it on <a href="${url}">GitHub</a>.`,
      callout: { NOTE: "Note", TIP: "Tip", IMPORTANT: "Important", WARNING: "Warning", CAUTION: "Caution" },
    },
  };

  // The visitor's choice, else the browser's language.
  let lang = (navigator.language || "").toLowerCase().startsWith("vi") ? "vi" : "en";
  try {
    const saved = localStorage.getItem("lang");
    if (saved === "vi" || saved === "en") lang = saved;
  } catch {}
  const subs = [];
  const page = document.body.dataset.page;

  function chrome() {
    const s = STR[lang];
    document.documentElement.lang = lang;
    const bar = document.getElementById("bar");
    bar.innerHTML = `
      <a class="brand" href="./"><img src="appicon.png" alt="">Ghostline</a>
      <nav class="pages" aria-label="Ghostline">
        ${Object.entries(s.pages).map(([k, v]) =>
          `<a href="${k === "index" ? "./" : k + ".html"}"${k === page ? ' aria-current="page"' : ""}>${v}</a>`).join("")}
      </nav>
      <span class="lang" role="group" aria-label="Language">
        <button type="button" data-lang="vi" aria-pressed="${lang === "vi"}">VI</button>
        <button type="button" data-lang="en" aria-pressed="${lang === "en"}">EN</button>
      </span>`;
    bar.querySelectorAll(".lang button").forEach((b) => b.addEventListener("click", () => setLang(b.dataset.lang)));
    document.getElementById("foot").innerHTML = `
      <p>${s.license} ${s.lawful}</p>
      <nav>
        <a href="${GH}">${s.source}</a>
        <a href="${GH}/releases">Releases</a>
        <a href="${GH}/issues">${s.issues}</a>
      </nav>`;
  }

  function setLang(l) {
    if (l === lang) return;
    lang = l;
    try { localStorage.setItem("lang", lang); } catch {}
    chrome();
    subs.forEach((fn) => fn(lang));
  }

  // GitHub's heading anchors, so the Markdown's own "#2-cài-đặt" links work.
  const slug = (text) => text.trim().toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, "").replace(/\s/g, "-");

  // Markdown from the repository to safe HTML. file is the Markdown's path in
  // the repository: relative images resolve against it on this site, other
  // relative links go to GitHub.
  function render(md, file) {
    const el = document.createElement("div");
    el.innerHTML = DOMPurify.sanitize(marked.parse(md, { gfm: true }));
    const dir = file.includes("/") ? file.slice(0, file.lastIndexOf("/") + 1) : "";
    const seen = {};
    el.querySelectorAll("h1, h2, h3, h4").forEach((h) => {
      let id = slug(h.textContent);
      if (seen[id] !== undefined) id += "-" + ++seen[id]; else seen[id] = 0;
      h.id = id;
    });
    el.querySelectorAll("img").forEach((img) => {
      const src = img.getAttribute("src") || "";
      if (!/^[a-z]+:|^\//i.test(src)) img.src = dir + src;
      img.loading = "lazy";
    });
    el.querySelectorAll("a[href]").forEach((a) => {
      const href = a.getAttribute("href");
      if (href.startsWith("#") || /^[a-z]+:/i.test(href)) return;
      // The README's disclaimer has its own page here.
      if (/README(\.vi)?\.md#(disclaimer|tuyên-bố)/.test(decodeURIComponent(href))) { a.href = "disclaimer.html"; return; }
      a.href = new URL(href, `${GH}/blob/main/${file}`).href;
    });
    el.querySelectorAll("table").forEach((t) => {
      const w = document.createElement("div");
      w.className = "table-wrap";
      t.replaceWith(w);
      w.append(t);
    });
    el.querySelectorAll("blockquote").forEach((q) => {
      const p = q.querySelector("p");
      const m = p && p.innerHTML.match(/^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*(<br>)?/);
      if (!m) return;
      p.innerHTML = p.innerHTML.slice(m[0].length);
      q.classList.add("callout");
      if (m[1] === "WARNING" || m[1] === "CAUTION") q.classList.add("warn");
      const title = document.createElement("span");
      title.className = "callout-title";
      title.textContent = STR[lang].callout[m[1]];
      q.prepend(title);
    });
    return el;
  }

  // The part of md under the heading that matches re, up to the next heading
  // of the same level; the heading itself is left out.
  function section(md, re) {
    const lines = md.split("\n");
    const start = lines.findIndex((l) => re.test(l));
    if (start < 0) return "";
    const level = lines[start].match(/^#+/)[0].length;
    let end = lines.length;
    for (let i = start + 1; i < lines.length; i++) {
      const h = lines[i].match(/^(#+)\s/);
      if (h && h[1].length <= level) { end = i; break; }
    }
    return lines.slice(start + 1, end).join("\n").trim();
  }

  // Fetches a Markdown file of the repository, as copied next to the pages.
  async function fetchMd(file) {
    const r = await fetch(file, { cache: "no-cache" });
    if (!r.ok) throw new Error(`${file}: HTTP ${r.status}`);
    return r.text();
  }

  // Shows loading, then build()'s element, or a link to GitHub on failure.
  async function fill(target, build, fallback) {
    const s = STR[lang];
    target.innerHTML = `<p class="loading">${s.loading}</p>`;
    try {
      const el = await build();
      target.replaceChildren(...el.childNodes);
      // The content came after the browser looked for #anchor.
      const hash = decodeURIComponent(location.hash.slice(1));
      if (hash) document.getElementById(hash)?.scrollIntoView();
    } catch (e) {
      console.error(e);
      target.innerHTML = `<p class="failed">${s.failed(fallback)}</p>`;
    }
  }

  chrome();
  return {
    REPO, GH,
    get lang() { return lang; },
    onLang(fn) { subs.push(fn); fn(lang); },
    render, section, fetchMd, fill, slug,
  };
})();
