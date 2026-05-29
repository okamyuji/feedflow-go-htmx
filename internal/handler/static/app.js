// feedflow Alpine.jsコンポーネントです。オーバーレイ開閉とテーマ切替とキーボードショートカットと自動既読を担います。

// csrfToken metaタグからCSRFトークンを取得します。
function csrfToken() {
  const meta = document.querySelector('meta[name="csrf-token"]');
  return meta ? meta.getAttribute("content") : "";
}

// postAction 状態変更系のアクションをCSRFトークン付きで送信します。
async function postAction(url, params) {
  const body = new URLSearchParams(params || {});
  await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-CSRF-Token": csrfToken(),
    },
    body: body.toString(),
  });
}

// feedflow ルートのAlpineデータを返します。初期テーマを引数で受け取ります。
function feedflow(initialTheme) {
  return {
    theme: initialTheme || "dark",
    overlayOpen: false,
    activeFeed: "",
    activeItem: "",

    init() {
      const saved = localStorage.getItem("feedflow-theme");
      if (saved === "dark" || saved === "light") {
        this.theme = saved;
        this.applyTheme();
      }
    },

    applyTheme() {
      document.documentElement.setAttribute("data-theme", this.theme);
    },

    toggleTheme() {
      this.theme = this.theme === "dark" ? "light" : "dark";
      this.applyTheme();
      localStorage.setItem("feedflow-theme", this.theme);
    },

    openOverlay(event, feedID, itemID) {
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
      const card = document.getElementById("item-" + itemID);
      if (card) {
        card.classList.add("is-read");
      }
    },

    closeOverlay() {
      this.overlayOpen = false;
      this.activeFeed = "";
      this.activeItem = "";
    },

    onOverlayScroll(event) {
      const el = event.target;
      const reachedEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
      if (reachedEnd && this.activeItem) {
        postAction(
          "/app/items/" + this.activeFeed + "/" + this.activeItem + "/read",
          { read: "true" }
        );
      }
    },

    focusNextCard(delta) {
      const cards = Array.from(document.querySelectorAll(".item-card"));
      if (cards.length === 0) {
        return;
      }
      const current = document.activeElement.closest(".item-card");
      let index = current ? cards.indexOf(current) : -1;
      index = Math.min(Math.max(index + delta, 0), cards.length - 1);
      const link = cards[index].querySelector(".item-open");
      if (link) {
        link.focus();
      }
    },

    onKey(event) {
      const tag = (event.target.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select") {
        return;
      }
      if (event.key === "Escape" && this.overlayOpen) {
        this.closeOverlay();
        return;
      }
      if (event.key === "j") {
        this.focusNextCard(1);
        event.preventDefault();
        return;
      }
      if (event.key === "k") {
        this.focusNextCard(-1);
        event.preventDefault();
        return;
      }
      const card = document.activeElement.closest(".item-card");
      if (!card) {
        return;
      }
      const feedID = card.getAttribute("data-feed");
      const itemID = card.getAttribute("data-item");
      if (event.key === "m") {
        postAction("/app/items/" + feedID + "/" + itemID + "/read", { read: "true" });
        card.classList.add("is-read");
        event.preventDefault();
      }
      if (event.key === "s") {
        postAction("/app/items/" + feedID + "/" + itemID + "/star", { starred: "true" });
        event.preventDefault();
      }
    },
  };
}

window.feedflow = feedflow;
