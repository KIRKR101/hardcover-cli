import argparse
import io
import json
import os
import stat
import sys
from collections import defaultdict
from datetime import date, datetime, timedelta
from pathlib import Path

# Ensure UTF-8 output on Windows
if sys.platform == "win32":
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")

import csv

import requests

API_URL = "https://api.hardcover.app/v1/graphql"
CONFIG_PATH = Path.home() / ".hardcover.json"
REQUEST_TIMEOUT = 30
LIBRARY_FETCH_LIMIT = 1000
JOURNAL_FETCH_LIMIT = 100

STATUS_MAP = {
    1: "Want to Read",
    2: "Currently Reading",
    3: "Read",
    4: "Paused",
    5: "Did Not Finish",
    6: "Ignored",
}

STATUS_SHORT = {"want": 1, "reading": 2, "read": 3, "paused": 4, "dnf": 5, "ignored": 6}


# ── Config ──────────────────────────────────────────────────────────────────


def load_token():
    if CONFIG_PATH.exists():
        return json.loads(CONFIG_PATH.read_text()).get("token")
    return os.environ.get("HARDCOVER_TOKEN")


def save_token(token):
    CONFIG_PATH.write_text(json.dumps({"token": token}, indent=2))
    if sys.platform != "win32":
        os.chmod(CONFIG_PATH, stat.S_IRUSR | stat.S_IWUSR)
    print(
        f"Token saved to {CONFIG_PATH}"
        + ("" if sys.platform == "win32" else " (permissions set to 600)")
    )


# ── API helpers ─────────────────────────────────────────────────────────────


def gql(query, variables=None):
    token = load_token()
    if not token:
        print("Error: No API token. Run `hardcover.py setup` first.", file=sys.stderr)
        sys.exit(1)
    headers = {"Authorization": f"Bearer {token}"}
    try:
        r = requests.post(
            API_URL,
            headers=headers,
            json={"query": query, "variables": variables},
            timeout=REQUEST_TIMEOUT,
        )
        r.raise_for_status()
    except requests.exceptions.Timeout:
        print("Error: Request to Hardcover API timed out.", file=sys.stderr)
        sys.exit(1)
    except requests.exceptions.HTTPError as e:
        status = e.response.status_code if e.response is not None else "?"
        if status == 401:
            print(
                "Error: Invalid or expired API token. Run `hardcover.py setup` again.",
                file=sys.stderr,
            )
        else:
            print(f"Error: Hardcover API returned HTTP {status}.", file=sys.stderr)
        sys.exit(1)
    except requests.exceptions.RequestException as e:
        print(f"Error: Could not reach Hardcover API ({e}).", file=sys.stderr)
        sys.exit(1)

    try:
        data = r.json()
    except ValueError:
        print("Error: Hardcover API returned a non-JSON response.", file=sys.stderr)
        sys.exit(1)

    if data.get("errors"):
        print(f"API errors: {data['errors']}", file=sys.stderr)
        sys.exit(1)
    return data["data"]


def get_me():
    q = """
    query { me { id username books_count } }
    """
    data = gql(q)
    me = data.get("me")
    if not me or not isinstance(me, list):
        print("Error: Could not retrieve user info from API.", file=sys.stderr)
        sys.exit(1)
    return me[0]


def print_or_json(args, payload, printer):
    """If --json was passed, dump raw payload; otherwise call printer()."""
    if getattr(args, "json", False):
        print(json.dumps(payload, indent=2))
    else:
        printer()


def fetch_all_user_books(user_id, fields, order_by="{ updated_at: desc }"):
    """Fetch all user_books for a user, paginating if necessary."""
    all_books = []
    offset = 0
    q = f"""
    query ($userId: Int!, $limit: Int!, $offset: Int!) {{
      user_books(
        where: {{ user_id: {{ _eq: $userId }} }}
        order_by: {order_by}
        limit: $limit
        offset: $offset
      ) {{
        {fields}
      }}
    }}
    """
    while True:
        result = gql(
            q, {"userId": user_id, "limit": LIBRARY_FETCH_LIMIT, "offset": offset}
        )
        books = result.get("user_books", [])
        all_books.extend(books)
        if len(books) < LIBRARY_FETCH_LIMIT:
            break
        offset += LIBRARY_FETCH_LIMIT
    return all_books


def effective_pages(user_book):
    """Return the page count for a user book, preferring the selected edition."""
    edition = user_book.get("edition")
    if edition:
        pages = edition.get("pages")
        if isinstance(pages, int) and pages > 0:
            return pages
    book = user_book.get("book", {})
    pages = book.get("pages")
    if isinstance(pages, int) and pages > 0:
        return pages
    return None


def fetch_read_books_total_pages(user_id):
    """Sum pages of books marked as read, paginating through large libraries."""
    total_pages = 0
    offset = 0
    q = """
    query ($userId: Int!, $limit: Int!, $offset: Int!) {
      user_books(
        where: { user_id: { _eq: $userId }, status_id: { _eq: 3 } }
        limit: $limit
        offset: $offset
      ) {
        edition { pages }
        book { pages }
      }
    }
    """
    while True:
        result = gql(
            q, {"userId": user_id, "limit": LIBRARY_FETCH_LIMIT, "offset": offset}
        )
        books = result.get("user_books", [])
        for ub in books:
            pages = effective_pages(ub)
            if isinstance(pages, int):
                total_pages += pages
        if len(books) < LIBRARY_FETCH_LIMIT:
            break
        offset += LIBRARY_FETCH_LIMIT
    return total_pages


def fetch_journals(user_id):
    """Fetch all reading journal events for a user, paginating."""
    q = """
    query ($userId: Int!, $limit: Int!, $offset: Int!) {
      reading_journals(
        where: {
          user_id: { _eq: $userId }
          event: { _in: ["progress_updated", "user_book_read_started", "user_book_read_finished"] }
        }
        order_by: { action_at: asc }
        limit: $limit
        offset: $offset
      ) {
        book_id
        action_at
        metadata
      }
    }
    """
    journals = []
    offset = 0
    while True:
        batch = gql(
            q,
            {"userId": user_id, "limit": JOURNAL_FETCH_LIMIT, "offset": offset},
        )["reading_journals"]
        journals.extend(batch)
        if len(batch) < JOURNAL_FETCH_LIMIT:
            break
        offset += JOURNAL_FETCH_LIMIT
    return journals


# ── Commands ────────────────────────────────────────────────────────────────


def cmd_setup(args):
    if args.token:
        print(
            "Warning: passing the token as a command-line argument may leave it in your "
            "shell history. Consider running `hardcover.py setup` with no argument instead.",
            file=sys.stderr,
        )
    token = args.token or input("Paste your Hardcover API token: ").strip()
    if not token:
        print("No token provided.", file=sys.stderr)
        sys.exit(1)
    save_token(token)


def cmd_whoami(args):
    me = get_me()

    def printer():
        print(f"  ID:       {me['id']}")
        print(f"  Username: {me['username']}")
        print(f"  Books:    {me['books_count']}")

    print_or_json(args, me, printer)


def cmd_library(args):
    me = get_me()
    status_filter = STATUS_SHORT.get(args.status) if args.status else None

    status_var = ", $statusId: Int" if status_filter is not None else ""
    status_where = (
        ", status_id: { _eq: $statusId }" if status_filter is not None else ""
    )
    q = f"""
    query ($userId: Int!, $limit: Int!, $offset: Int!{status_var}) {{
      user_books(
        where: {{ user_id: {{ _eq: $userId }}{status_where} }}
        order_by: {{ date_added: desc }}
        limit: $limit
        offset: $offset
      ) {{
        id
        status_id
        rating
        date_added
        owned
        book {{
          id
          title
          pages
          contributions {{
            author {{ name }}
          }}
        }}
      }}
    }}
    """
    variables = {
        "userId": me["id"],
        "limit": args.limit,
        "offset": args.offset,
    }
    if status_filter is not None:
        variables["statusId"] = status_filter

    result = gql(q, variables)
    books = result["user_books"]

    def printer():
        if not books:
            print(
                "No books found."
                if not args.status
                else f"No books with status '{args.status}'."
            )
            return
        print(f"{'#':<4} {'Status':<18} {'Rating':<7} {'Title':<45} {'Authors'}")
        print("─" * 100)
        for i, ub in enumerate(books, 1):
            status = STATUS_MAP.get(ub["status_id"], "?")
            rating = f"{ub['rating']:.1f}" if ub.get("rating") else "—"
            authors = ", ".join(
                c["author"]["name"] for c in ub["book"]["contributions"][:2]
            )
            if len(ub["book"]["contributions"]) > 2:
                authors += " et al."
            title = ub["book"]["title"][:44]
            owned = " [own]" if ub["owned"] else ""
            print(f"{i:<4} {status:<18} {rating:<7} {title:<45} {authors}{owned}")
        print(f"\n  Showing {len(books)} books (offset={args.offset})")

    print_or_json(args, books, printer)


def cmd_stats(args):
    me = get_me()
    user_id = me["id"]

    # Counts by status, average rating, and active goals. Total pages are summed
    # separately because Hasura cannot aggregate nested relationship columns.
    q = """
    query ($userId: Int!) {
      total: user_books_aggregate(where: { user_id: { _eq: $userId } }) {
        aggregate { count }
      }
      read: user_books_aggregate(where: { user_id: { _eq: $userId }, status_id: { _eq: 3 } }) {
        aggregate { count }
      }
      reading: user_books_aggregate(where: { user_id: { _eq: $userId }, status_id: { _eq: 2 } }) {
        aggregate { count }
      }
      want: user_books_aggregate(where: { user_id: { _eq: $userId }, status_id: { _eq: 1 } }) {
        aggregate { count }
      }
      dnf: user_books_aggregate(where: { user_id: { _eq: $userId }, status_id: { _eq: 5 } }) {
        aggregate { count }
      }
      rated: user_books_aggregate(where: { user_id: { _eq: $userId }, rating: { _gt: 0 } }) {
        aggregate {
          count
          avg { rating }
        }
      }
      goals(where: { user_id: { _eq: $userId }, archived: { _eq: false } }) {
        id
        metric
        goal
        progress
        state
        start_date
        end_date
      }
    }
    """
    data = gql(q, {"userId": user_id})

    total_count = data["total"]["aggregate"]["count"]
    read_count = data["read"]["aggregate"]["count"]
    total_pages = fetch_read_books_total_pages(user_id)
    reading_count = data["reading"]["aggregate"]["count"]
    want_count = data["want"]["aggregate"]["count"]
    dnf_count = data["dnf"]["aggregate"]["count"]
    rated_count = data["rated"]["aggregate"]["count"]
    avg_rating = (data["rated"]["aggregate"].get("avg") or {}).get("rating") or 0
    goals = data["goals"]

    def printer():
        print(f"\n  Library Stats for {me['username']}")
        print("  " + "─" * 40)
        print(f"  Total books:       {total_count}")
        print(f"  Read:              {read_count}")
        print(f"  Currently reading: {reading_count}")
        print(f"  Want to read:      {want_count}")
        print(f"  DNF:               {dnf_count}")
        print(f"  Total pages read:  ~{total_pages:,}")
        if rated_count:
            print(f"  Avg rating:        {avg_rating:.2f} ({rated_count} rated)")
        print()

        if goals:
            print("  Active Goals")
            print("  " + "─" * 40)
            for g in goals:
                pct = (float(g["progress"]) / g["goal"] * 100) if g["goal"] else 0
                bar_len = 20
                filled = int(pct / 100 * bar_len)
                bar = "█" * filled + "░" * (bar_len - filled)
                print(
                    f"  {g['metric']}: {g['progress']}/{g['goal']}  [{bar}] {pct:.0f}%"
                )
            print()

    print_or_json(args, data, printer)


def cmd_progress(args):
    me = get_me()

    q = """
    query ($userId: Int!) {
      user_books(
        where: { user_id: { _eq: $userId }, status_id: { _eq: 2 } }
        order_by: { updated_at: desc }
      ) {
        id
        user_book_reads(
          where: { finished_at: { _is_null: true } }
          order_by: [{ started_at: desc_nulls_last }, { id: desc }]
          limit: 1
        ) {
          progress_pages
        }
        edition { pages }
        book {
          id
          title
          pages
        }
      }
    }
    """
    data = gql(q, {"userId": me["id"]})
    books = data["user_books"]

    def printer():
        if not books:
            print("No books currently being read.")
            return
        print(f"\n  Currently Reading ({len(books)} books)")
        print("  " + "─" * 60)
        for ub in books:
            book = ub["book"]
            total = effective_pages(ub)
            reads = ub.get("user_book_reads", [])
            current = reads[0]["progress_pages"] if reads else 0
            total_int = total if isinstance(total, int) else 0
            pct = (current / total_int * 100) if total_int else 0
            bar_len = 25
            filled = int(pct / 100 * bar_len)
            bar = "█" * filled + "░" * (bar_len - filled)
            total_display = total if total_int else "?"
            print(f"\n  {book['title']}")
            print(f"    {current}/{total_display} pages  [{bar}] {pct:.0f}%")
        print()

    print_or_json(args, books, printer)


def cmd_log(args):
    """Log reading progress, status change, or rating for a book."""
    if args.percent is not None and not (0 <= args.percent <= 100):
        print("Error: --percent must be between 0 and 100.", file=sys.stderr)
        sys.exit(1)
    if args.rating is not None and not (0 <= args.rating <= 5):
        print("Error: --rating must be between 0 and 5.", file=sys.stderr)
        sys.exit(1)

    me = get_me()

    user_book_id = args.user_book_id
    edition_id = None
    if user_book_id is None:
        # Search by title in library (paginated for large libraries)
        data = fetch_all_user_books(
            me["id"], "id\nedition_id\nedition { pages }\nbook { id title pages }"
        )
        # Fuzzy match: try exact, then substring
        query = args.book.lower()
        matches = []
        for ub in data:
            title = ub["book"]["title"].lower()
            if query == title:
                matches = [ub]
                break
            elif query in title:
                matches.append(ub)

        if not matches:
            print(f"No book matching '{args.book}' in your library.")
            sys.exit(1)
        if len(matches) > 1:
            print(f"Multiple matches for '{args.book}':")
            for ub in matches:
                print(f"  [{ub['id']}] {ub['book']['title']}")
            print("Use the user_book_id directly: hardcover.py log --id <id> ...")
            sys.exit(1)
        selected_ub = matches[0]
        user_book_id = selected_ub["id"]
        edition_id = selected_ub.get("edition_id")
        book_title = selected_ub["book"]["title"]
        book_pages = effective_pages(selected_ub)
    else:
        book_title = f"user_book #{user_book_id}"
        book_pages = None
        # Fetch pages/edition if needed for percent or logging
        if args.percent is not None or args.pages is not None:
            ub_q = """
            query ($id: Int!) {
              user_books_by_pk(id: $id) {
                edition_id
                edition { pages }
                book { pages }
              }
            }
            """
            ub_data = gql(ub_q, {"id": user_book_id})
            selected_ub = ub_data.get("user_books_by_pk")
            if not selected_ub:
                print(f"Error: user_book #{user_book_id} not found.", file=sys.stderr)
                sys.exit(1)
            edition_id = selected_ub.get("edition_id")
            book_pages = effective_pages(selected_ub)

    if args.percent is not None:
        if not book_pages:
            print(
                f"  Could not determine page count for '{book_title}'. Use --pages instead."
            )
            sys.exit(1)
        args.pages = round(book_pages * args.percent / 100)

    if args.pages is not None and args.pages < 0:
        print("Error: --pages cannot be negative.", file=sys.stderr)
        sys.exit(1)

    if args.pages is not None:
        read_q = """
        query ($user_book_id: Int!) {
          user_book_reads(
            where: {
              user_book_id: { _eq: $user_book_id }
              finished_at: { _is_null: true }
            }
            order_by: [{ started_at: desc_nulls_last }, { id: desc }]
            limit: 1
          ) {
            id
          }
        }
        """
        read_data = gql(read_q, {"user_book_id": user_book_id})
        reads = read_data["user_book_reads"]
        read_input = {"progress_pages": args.pages}
        if edition_id is not None:
            read_input["edition_id"] = edition_id

        if reads:
            read_id = reads[0]["id"]
            m = """
            mutation ($id: Int!, $object: DatesReadInput!) {
              update_user_book_read(id: $id, object: $object) {
                id
              }
            }
            """
            gql(m, {"id": read_id, "object": read_input})
        else:
            m = """
            mutation ($user_book_id: Int!, $user_book_read: DatesReadInput!) {
              insert_user_book_read(user_book_id: $user_book_id, user_book_read: $user_book_read) {
                id
              }
            }
            """
            gql(
                m,
                {
                    "user_book_id": user_book_id,
                    "user_book_read": read_input,
                },
            )

        print(f"  Logged {args.pages} pages for '{book_title}'")

    if args.status is not None:
        status_id = STATUS_SHORT[args.status]
        m = """
        mutation ($id: Int!, $object: UserBookUpdateInput!) {
          update_user_book(id: $id, object: $object) {
            id
          }
        }
        """
        gql(m, {"id": user_book_id, "object": {"status_id": status_id}})
        print(f"  Updated '{book_title}' status to '{STATUS_MAP[status_id]}'")

    if args.rating is not None:
        m = """
        mutation ($id: Int!, $object: UserBookUpdateInput!) {
          update_user_book(id: $id, object: $object) {
            id
          }
        }
        """
        gql(m, {"id": user_book_id, "object": {"rating": args.rating}})
        print(f"  Rated '{book_title}' {args.rating}/5")

    if args.pages is None and args.status is None and args.rating is None:
        print("Nothing to log. Use --pages, --status, or --rating.")
        sys.exit(1)


def cmd_search(args):
    q = """
    query ($query: String!, $perPage: Int!) {
      search(query: $query, query_type: "Book", per_page: $perPage, page: 1) {
        results
      }
    }
    """
    data = gql(q, {"query": args.query, "perPage": args.limit})
    hits = data["search"]["results"].get("hits", [])

    def printer():
        if not hits:
            print(f"No results for '{args.query}'.")
            return
        print(f'\n  Search: "{args.query}"')
        print("  " + "─" * 80)
        for i, hit in enumerate(hits, 1):
            r = hit.get("document", hit)
            if isinstance(r, str):
                r = json.loads(r)
            title = r.get("title", "?")
            author = r.get("author_names", "?")
            if isinstance(author, list):
                author = ", ".join(author)
            pages = r.get("pages", "?")
            rating = r.get("rating", "?")
            year = r.get("release_year", "?")
            print(f"  {i:>3}. {title}")
            print(f"       by {author}  |  {pages}p  |  {year}  |  rating {rating}")
        print()

    print_or_json(args, hits, printer)


def cmd_goals(args):
    me = get_me()
    archived_where = "" if args.all else ", archived: { _eq: false }"
    q = f"""
    query ($userId: Int!) {{
      goals(
        where: {{ user_id: {{ _eq: $userId }}{archived_where} }}
        order_by: {{ start_date: desc }}
      ) {{
        id
        metric
        goal
        progress
        state
        start_date
        end_date
        description
      }}
    }}
    """
    data = gql(q, {"userId": me["id"]})
    goals = data["goals"]

    def printer():
        if not goals:
            print("No reading goals found.")
            return
        print("\n  Reading Goals")
        print("  " + "─" * 55)
        for g in goals:
            pct = (float(g["progress"]) / g["goal"] * 100) if g["goal"] else 0
            bar_len = 20
            filled = int(pct / 100 * bar_len)
            bar = "█" * filled + "░" * (bar_len - filled)
            state_tag = {
                "active": "[active]",
                "completed": "[done]",
                "failed": "[failed]",
            }.get(g["state"], "[?]")
            print(f"\n  {state_tag}  {g['metric'].title()} Goal")
            print(f"     {g['progress']}/{g['goal']}  [{bar}] {pct:.0f}%")
            print(f"     {g['start_date']} → {g['end_date']}  ({g['state']})")
            if g.get("description"):
                print(f"     {g['description']}")
        print()

    print_or_json(args, goals, printer)


def cmd_export(args):
    """Export reading journal events to CSV."""
    me = get_me()
    user_id = me["id"]

    all_books = fetch_all_user_books(user_id, "book { id title }")
    titles = {b["book"]["id"]: b["book"]["title"] for b in all_books}

    journals = fetch_journals(user_id)

    events = []
    daily = defaultdict(lambda: defaultdict(int))

    for j in journals:
        m = j.get("metadata") or {}
        current = m.get("progress_pages")
        pages = (
            current - (m.get("progress_pages_was") or 0) if current is not None else 0
        )
        if pages <= 0:
            continue
        dt = (
            datetime.fromisoformat(j["action_at"].replace("Z", "+00:00"))
            .date()
            .isoformat()
        )
        book = titles.get(j["book_id"], f"book_id {j['book_id']}")
        events.append(
            {
                "book": book,
                "date": dt,
                "timestamp": j["action_at"],
                "cumulative_pages": current,
                "pages_read": pages,
            }
        )
        daily[dt][book] += pages

    out = args.output
    with open(out, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(
            f, ["book", "date", "timestamp", "cumulative_pages", "pages_read"]
        )
        w.writeheader()
        w.writerows(events)

    print(f"Wrote {len(events)} events to {out}")
    print("\nPages read per day:")
    for dt, books in sorted(daily.items()):
        print(f"  {dt}: {sum(books.values())}")
        for book, pages in books.items():
            print(f"    {book}: {pages}")


def cmd_daily(args):
    """Show daily reading log with pages per book, cumulative progress, and relative volume bars."""
    me = get_me()
    user_id = me["id"]

    all_books = fetch_all_user_books(user_id, "book { id title pages }")
    book_info = {}
    for b in all_books:
        bid = b["book"]["id"]
        book_info[bid] = {
            "title": b["book"]["title"],
            "pages": b["book"].get("pages"),
        }

    journals = fetch_journals(user_id)

    daily = {}
    for j in journals:
        m = j.get("metadata") or {}
        current = m.get("progress_pages")
        pages = (
            current - (m.get("progress_pages_was") or 0) if current is not None else 0
        )
        if pages <= 0:
            continue

        dt = datetime.fromisoformat(j["action_at"].replace("Z", "+00:00")).date()
        bid = j["book_id"]
        info = book_info.get(bid, {"title": f"book_id {bid}", "pages": None})

        day = daily.setdefault(dt, {})
        entry = day.setdefault(
            bid,
            {
                "pages": 0,
                "cumulative": 0,
                "total_pages": info["pages"],
                "title": info["title"],
            },
        )
        entry["pages"] += pages
        entry["cumulative"] = current

    today = date.today()
    cutoff = today - timedelta(days=args.days - 1)
    filtered = sorted(
        [(d, books) for d, books in daily.items() if cutoff <= d <= today],
        key=lambda x: x[0],
    )

    if not filtered:

        def empty_printer():
            print(f"  No reading activity in the last {args.days} days.")

        print_or_json(args, [], empty_printer)
        return

    weekday_names = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]

    json_result = []
    for dt, books in filtered:
        day_total = sum(b["pages"] for b in books.values())
        books_list = []
        for bid, b in sorted(books.items(), key=lambda x: x[1]["pages"], reverse=True):
            books_list.append(
                {
                    "title": b["title"],
                    "pages": b["pages"],
                    "cumulative": b["cumulative"],
                    "total_book_pages": b["total_pages"],
                }
            )
        json_result.append(
            {
                "date": dt.isoformat(),
                "total_pages": day_total,
                "books": books_list,
            }
        )

    def printer():
        total_all = 0
        print(f"\n  Reading Log (last {args.days} days)")
        print("  " + "─" * 80)

        for entry in json_result:
            day_total = entry["total_pages"]
            total_all += day_total
            wd = weekday_names[datetime.fromisoformat(entry["date"]).weekday()]
            print(f"\n  {entry['date']} {wd}  {day_total} pages")

            for b in entry["books"]:
                cumul = b["cumulative"]
                total = b["total_book_pages"]
                cumul_str = f"{cumul}/{total}" if total else str(cumul)
                dots = max(2, 50 - len(b["title"]))
                print(
                    f"    {b['title']} {'.' * dots} {b['pages']}p  (cumulative {cumul_str})"
                )

        avg = total_all // len(json_result) if json_result else 0
        print("\n  " + "─" * 80)
        print(
            f"  Total: {total_all} pages across {len(json_result)} days  (avg {avg}/day)"
        )

    print_or_json(args, json_result, printer)


# ── CLI ─────────────────────────────────────────────────────────────────────


def add_json_flag(p):
    p.add_argument(
        "--json", action="store_true", help="Output raw JSON instead of formatted text"
    )


def main():
    parser = argparse.ArgumentParser(
        prog="hardcover",
        description="Manage your hardcover.app library from the terminal.",
    )
    sub = parser.add_subparsers(dest="command", help="Available commands")

    # setup
    p = sub.add_parser("setup", help="Save your API token")
    p.add_argument(
        "token",
        nargs="?",
        help="API token (or paste when prompted; avoid this on shared machines)",
    )
    p.set_defaults(func=cmd_setup)

    # whoami
    p = sub.add_parser("whoami", help="Show current user info")
    add_json_flag(p)
    p.set_defaults(func=cmd_whoami)

    # library
    p = sub.add_parser("library", help="List books in your library")
    p.add_argument(
        "-s", "--status", choices=list(STATUS_SHORT), help="Filter by status"
    )
    p.add_argument(
        "-l", "--limit", type=int, default=25, help="Max books to show (default: 25)"
    )
    p.add_argument("-o", "--offset", type=int, default=0, help="Offset for pagination")
    add_json_flag(p)
    p.set_defaults(func=cmd_library)

    # stats
    p = sub.add_parser("stats", help="Show reading statistics")
    add_json_flag(p)
    p.set_defaults(func=cmd_stats)

    # progress
    p = sub.add_parser("progress", help="Show current reading progress")
    add_json_flag(p)
    p.set_defaults(func=cmd_progress)

    # search
    p = sub.add_parser("search", help="Search for books on Hardcover")
    p.add_argument("query", help="Search query")
    p.add_argument(
        "-l", "--limit", type=int, default=10, help="Max results (default: 10)"
    )
    add_json_flag(p)
    p.set_defaults(func=cmd_search)

    # goals
    p = sub.add_parser("goals", help="Show reading goals")
    p.add_argument("--all", action="store_true", help="Include archived goals")
    add_json_flag(p)
    p.set_defaults(func=cmd_goals)

    # export
    p = sub.add_parser("export", help="Export reading journal to CSV")
    p.add_argument(
        "-o",
        "--output",
        default="hardcover_export.csv",
        help="Output file (default: hardcover_export.csv)",
    )
    p.set_defaults(func=cmd_export)

    # daily
    p = sub.add_parser("daily", help="Show daily reading log")
    p.add_argument(
        "-d", "--days", type=int, default=7, help="Number of days to show (default: 7)"
    )
    add_json_flag(p)
    p.set_defaults(func=cmd_daily)

    # log
    p = sub.add_parser("log", help="Log reading progress, status, or rating")
    p.add_argument("book", nargs="?", help="Book title (fuzzy match) or use --id")
    p.add_argument("--id", dest="user_book_id", type=int, help="user_book ID directly")
    p.add_argument("--pages", type=int, help="Log cumulative pages read")
    p.add_argument(
        "--percent", type=float, help="Log as percentage of total pages (0-100)"
    )
    p.add_argument("--status", choices=list(STATUS_SHORT), help="Update status")
    p.add_argument("--rating", type=float, help="Rate the book (0-5, supports halves)")
    p.set_defaults(func=cmd_log)

    args = parser.parse_args()
    if not args.command:
        parser.print_help()
        sys.exit(1)

    if args.command == "log" and not args.user_book_id and not args.book:
        parser.error("log requires a book title or --id")

    args.func(args)


if __name__ == "__main__":
    main()
