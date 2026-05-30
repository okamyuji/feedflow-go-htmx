// feedflow Alpine.jsコンポーネントです。オーバーレイ開閉とテーマ切替とキーボードショートカットと自動既読を担います。
// CSPビルドのAlpineで動かすため、テンプレートのインライン式は使わず、ここで登録した
// プロパティとメソッドだけを参照します。script-srcはselfに限定しunsafe-evalを使いません。

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

// cardActionData クリック起点の要素から所属カードのフィードIDと記事IDを取り出します。
function cardActionData(target) {
  const card = target.closest(".item-card");
  if (!card) {
    return { feedID: "", itemID: "", card: null };
  }
  return {
    feedID: card.getAttribute("data-feed") || "",
    itemID: card.getAttribute("data-item") || "",
    card,
  };
}

// registerFeedflow Alpineの初期化時にfeedflowコンポーネントを登録します。
function registerFeedflow() {
  window.Alpine.data("feedflow", () => ({
    theme: "dark",
    overlayOpen: false,
    activeFeed: "",
    activeItem: "",
    sidebarOpen: true,
    isMobile: false,
    autoRead: true,
    markedRead: null,
    feedFilter: "",
    markMenuOpen: false,
    treeScrollTop: 0,

    init() {
      this.markedRead = new Set();
      const saved = localStorage.getItem("feedflow-theme");
      const initial = document.documentElement.getAttribute("data-theme");
      if (saved === "dark" || saved === "light") {
        this.theme = saved;
      } else if (initial === "dark" || initial === "light") {
        this.theme = initial;
      }
      this.applyTheme();

      // モバイルではツリーをオフキャンバス・ドロワーにするため既定は閉、デスクトップは保存値に従います。
      const mql = window.matchMedia("(max-width: 48rem)");
      this.isMobile = mql.matches;
      if (this.isMobile) {
        this.sidebarOpen = false;
      } else if (localStorage.getItem("feedflow-sidebar") === "closed") {
        this.sidebarOpen = false;
      }
      // 画面幅が変わったらモバイル判定を更新します。
      // モバイルへ入るとドロワーは閉、デスクトップへ戻ると保存済みの開閉設定を尊重します。
      mql.addEventListener("change", (e) => {
        this.isMobile = e.matches;
        if (e.matches) {
          this.sidebarOpen = false;
        } else {
          this.sidebarOpen = localStorage.getItem("feedflow-sidebar") !== "closed";
        }
      });
      // 右ペインの本文が差し替わったとき(フィード選択・一括既読・設定遷移)の処理です。
      // main-pane自体が差し替わった場合だけを対象にします。記事カード個別の自動既読スワップ
      // (#item-XへのouterHTML)はmain-pane内で起きてイベントが伝播してくるため、ここで除外します。
      const main = document.getElementById("main-pane");
      if (main) {
        main.addEventListener("htmx:afterSwap", (event) => {
          if (event.target !== main) {
            return;
          }
          // 新しいフィードの記事一覧を必ず最上部から見せます。innerHTMLスワップではブラウザが
          // 直前のスクロール位置を保持してしまい、記事の先頭が隠れることがあるため明示的に戻します。
          main.scrollTop = 0;
          // モバイルではツリーのリンクをタップして本文が入れ替わったらドロワーを閉じます。
          if (this.isMobile) {
            this.sidebarOpen = false;
          }
        });
      }

      // 左サイドバー(tree-pane)のスクロール位置を、tree-paneを差し替えるHTMXスワップをまたいで保持します。
      // フィード選択(OOBでtree-paneを同梱)・購読追加/解除(tree-paneを直接差し替え)のたびにコンテナごと
      // 作り直され、何もしないと先頭に戻って今どれを選んでいるか分からなくなるためです。
      // ただしtree-paneに無関係なスワップ(自動既読でtree-paneを含まないカード差し替えやブックマーク操作など)で
      // スクロール位置を巻き戻さないよう、退避と復元はtree-paneが差し替わるスワップに限定します。
      // 判定はスワップ対象自体がtree-paneか、レスポンスにtree-paneのOOBが含まれるかで行います。
      // htmxはbeforeSwapをswap()やOOB処理より前に発火するので、差し替え前の値を確実に退避できます。
      let restoreTreeScroll = false;
      document.body.addEventListener("htmx:beforeSwap", (event) => {
        const tree = document.getElementById("tree-pane");
        const detail = event.detail || {};
        const target = detail.target;
        const response = detail.serverResponse || "";
        const swapsTree =
          (!!target && (target.id === "tree-pane" || (!!tree && target.contains(tree)))) ||
          response.indexOf('id="tree-pane"') !== -1;
        restoreTreeScroll = swapsTree && !!tree;
        if (restoreTreeScroll) {
          this.treeScrollTop = tree.scrollTop;
        }
      });
      // 差し替え後のtree-paneへ退避値を復元します。要素が作り直されてもidで取り直して設定します。
      // 購読解除で内容が縮む場合はブラウザがclampするため、削除位置の近辺が保たれます。
      document.body.addEventListener("htmx:afterSettle", () => {
        if (!restoreTreeScroll) {
          return;
        }
        restoreTreeScroll = false;
        const tree = document.getElementById("tree-pane");
        if (tree) {
          tree.scrollTop = this.treeScrollTop;
        }
      });
      this.autoRead = document.body.getAttribute("data-auto-read") !== "false";
    },

    get themeLabel() {
      return this.theme === "dark" ? "ライト" : "ダーク";
    },

    // applyFeedFilter feedFilterの文字列でサイドバーのフィード行を絞り込みます。
    // フィード一覧は既にDOMにあるため、ネットワークを介さずクライアントだけで表示と非表示を切り替えます。
    // 固定ナビ(すべて/既読/スター/あとで読む)は対象にせず常に表示します。一致なしは案内を出します。
    applyFeedFilter() {
      const q = (this.feedFilter || "").trim().toLowerCase();
      const items = document.querySelectorAll(".tree-feeds .tree-feed");
      let visible = 0;
      items.forEach((li) => {
        const label = (li.getAttribute("data-label") || "").toLowerCase();
        const match = q === "" || label.indexOf(q) !== -1;
        li.style.display = match ? "" : "none";
        if (match) {
          visible += 1;
        }
      });
      const empty = document.querySelector(".tree-feed-empty");
      if (empty) {
        empty.style.display = q !== "" && visible === 0 ? "" : "none";
      }
    },

    // onFeedFilter 入力のたびに即時で絞り込みます。値はイベントから直接読み、空にした時点で全表示へ戻します。
    onFeedFilter(event) {
      this.feedFilter = event.target.value;
      this.applyFeedFilter();
    },

    // clearFeedFilter 絞り込みを解除して全フィードを再表示します。クリアボタンとEscapeキーから呼びます。
    clearFeedFilter() {
      this.feedFilter = "";
      this.applyFeedFilter();
    },

    // toggleMarkMenu 一括既読のサブメニュー(すべてのフィードを既読)の開閉を切り替えます。
    toggleMarkMenu() {
      this.markMenuOpen = !this.markMenuOpen;
    },

    // closeMarkMenu 一括既読のサブメニューを閉じます。外側クリックや項目選択で呼びます。
    closeMarkMenu() {
      this.markMenuOpen = false;
    },

    get sidebarClass() {
      return this.sidebarOpen ? "sidebar-open" : "sidebar-collapsed";
    },

    toggleSidebar() {
      this.sidebarOpen = !this.sidebarOpen;
      // デスクトップの開閉のみ記憶します。モバイルのドロワーは毎回閉じた状態から始めます。
      if (!this.isMobile) {
        localStorage.setItem("feedflow-sidebar", this.sidebarOpen ? "open" : "closed");
      }
    },

    closeSidebar() {
      this.sidebarOpen = false;
    },

    applyTheme() {
      document.documentElement.setAttribute("data-theme", this.theme);
    },

    toggleTheme() {
      this.theme = this.theme === "dark" ? "light" : "dark";
      this.applyTheme();
      localStorage.setItem("feedflow-theme", this.theme);
    },

    openOverlay(event) {
      const { feedID, itemID, card } = cardActionData(event.currentTarget);
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
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
      if (!this.autoRead) {
        return;
      }
      const el = event.target;
      const reachedEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
      if (reachedEnd && this.activeItem) {
        postAction(
          "/app/items/" + this.activeFeed + "/" + this.activeItem + "/read",
          { read: "true" }
        );
      }
    },

    // onListScroll 記事一覧をスクロールしたとき上端より上へ流れた未読カードを既読にします。
    // 自動既読がオフのときは何もしません。同じ記事を二重送信しないようmarkedReadで記録します。
    // htmx.ajaxで既読化することでカードの再描画と未読数のout-of-band更新を同時に行います。
    onListScroll() {
      if (!this.autoRead) {
        return;
      }
      const bar = document.querySelector(".app-bar");
      const threshold = Math.max(0, bar ? bar.getBoundingClientRect().bottom : 0);
      const cards = document.querySelectorAll(".item-card:not(.is-read)");
      cards.forEach((card) => {
        if (card.getBoundingClientRect().bottom > threshold) {
          return;
        }
        const feedID = card.getAttribute("data-feed");
        const itemID = card.getAttribute("data-item");
        if (!feedID || !itemID) {
          return;
        }
        const key = feedID + "/" + itemID;
        if (this.markedRead.has(key)) {
          return;
        }
        this.markedRead.add(key);
        card.classList.add("is-read");
        window.htmx.ajax("POST", "/app/items/" + feedID + "/" + itemID + "/read", {
          source: card,
          target: "#item-" + itemID,
          swap: "outerHTML",
          values: { read: "true" },
        });
      });
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
      if (event.key === "b") {
        const bookmarkBtn = card.querySelector(".bookmark-btn");
        if (bookmarkBtn) {
          bookmarkBtn.click();
        }
        event.preventDefault();
      }
    },
  }));
}

document.addEventListener("alpine:init", registerFeedflow);
