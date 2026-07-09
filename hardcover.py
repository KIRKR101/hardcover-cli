import argparse
import io
import json
import os
import stat
import sys
from collections import defaultdict
from datetime import date, datetime, timedelta
from pathlib import Path

import csv

import requests

# Ensure UTF-8 output and ANSI colors on Windows
if sys.platform == "win32":
    os.system("")  # Enables ANSI escape sequences in Windows 10+ CMD/PowerShell
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")

VERSION = "0.1.0"
API_URL = "https://api.hardcover.app/v1/graphql"
CONFIG_PATH = Path.home() / ".hardcover.json"
REQUEST_TIMEOUT = 30
LIBRARY_FETCH_LIMIT = 1000
JOURNAL_FETCH_LIMIT = 100

DISABLE_COLOR = "NO_COLOR" in os.environ

STATUS_MAP = {
    1: "Want to Read",
    2: "Currently Reading",
    3: "Read",
    4: "Paused",
    5: "Did Not Finish",
    6: "Ignored",
}

STATUS_SHORT = {"want": 1, "reading": 2, "read": 3, "paused": 4, "dnf": 5, "ignored": 6}


# ── Terminal Styling ────────────────────────────────────────────────────────


class Term:
    RESET = "\033[0m"
    BOLD = "\033[1m"
    DIM = "\033[2m"
    UNDERLINE = "\033[4m"

    # Colors
    RED = "\033[31m"
    GREEN = "\033[32m"
    YELLOW = "\033[33m"
    BLUE = "\033[34m"
    MAGENTA = "\033[35m"
    CYAN = "\033[36m"
    WHITE = "\033[37m"

    # Bright Colors
    B_GREEN = "\033[92m"
    B_YELLOW = "\033[93m"
    B_CYAN = "\033[96m"


def _status_color(status_id):
    colors = {
        1: Term.CYAN,
        2: Term.B_YELLOW,
        3: Term.B_GREEN,
        4: Term.MAGENTA,
        5: Term.RED,
        6: Term.DIM,
    }
    return colors.get(status_id, Term.RESET)

def _disable_color():
    for attr in list(vars(Term)):
        if not attr.startswith("_") and isinstance(getattr(Term, attr), str):
            setattr(Term, attr, "")


if DISABLE_COLOR:
    _disable_color()


def make_progress_bar(pct, width=20):
    """Generate high-resolution Unicode progress bar."""
    pct = max(0.0, min(100.0, pct))

    if pct >= 75:
        color = Term.B_GREEN
    elif pct >= 25:
        color = Term.B_YELLOW
    else:
        color = Term.RED

    filled_width = (pct / 100.0) * width
    filled_chars = int(filled_width)
    fraction = filled_width - filled_chars

    # Block sub-characters for granular step display
    blocks = [" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉"]
    char_idx = int(fraction * 8)

    bar = f"{color}" + "█" * filled_chars
    if filled_chars < width:
        bar += blocks[char_idx] + " " * (width - filled_chars - 1)
    bar += f"{Term.RESET}"
    return bar


def pad_styled(text, styled_text, width):
    """Pad styled string layout taking into account unstyled character length."""
    plain_len = len(text)
    padding = " " * max(0, width - plain_len)
    return styled_text + padding


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
        print(
            f"{Term.RED}Error: No API token. Run `hardcover.py setup` first.{Term.RESET}",
            file=sys.stderr,
        )
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
        print(
            f"{Term.RED}Error: Request to Hardcover API timed out.{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)
    except requests.exceptions.HTTPError as e:
        status = e.response.status_code if e.response is not None else "?"
        if status == 401:
            print(
                f"{Term.RED}Error: Invalid or expired API token. Run `hardcover.py setup` again.{Term.RESET}",
                file=sys.stderr,
            )
        else:
            print(
                f"{Term.RED}Error: Hardcover API returned HTTP {status}.{Term.RESET}",
                file=sys.stderr,
            )
        sys.exit(1)
    except requests.exceptions.RequestException as e:
        print(
            f"{Term.RED}Error: Could not reach Hardcover API ({e}).{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)

    try:
        data = r.json()
    except ValueError:
        print(
            f"{Term.RED}Error: Hardcover API returned a non-JSON response.{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)

    if data.get("errors"):
        print(f"{Term.RED}API errors: {data['errors']}{Term.RESET}", file=sys.stderr)
        sys.exit(1)
    return data["data"]


def get_me():
    q = """
    query { me { id username books_count } }
    """
    data = gql(q)
    me = data.get("me")
    if not me or not isinstance(me, list):
        print(
            f"{Term.RED}Error: Could not retrieve user info from API.{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)
    return me[0]


def print_or_json(args, payload, printer):
    """If --json was passed, dump raw payload; otherwise call printer()."""
    if getattr(args, "json", False):
        print(json.dumps(payload, indent=2))
    else:
        printer()


def fetch_all_user_books(user_id, fields, order_by="{ updated_at: desc }", title_filter=None):
    """Fetch all user_books for a user, paginating if necessary."""
    all_books = []
    offset = 0
    title_where = ', book: { title: { _ilike: $titleFilter } }' if title_filter else ""
    title_var = ", $titleFilter: String" if title_filter else ""
    q = f"""
    query ($userId: Int!, $limit: Int!, $offset: Int!{title_var}) {{
      user_books(
        where: {{ user_id: {{ _eq: $userId }}{title_where} }}
        order_by: {order_by}
        limit: $limit
        offset: $offset
      ) {{
        {fields}
      }}
    }}
    """
    variables = {"userId": user_id, "limit": LIBRARY_FETCH_LIMIT, "offset": offset}
    if title_filter:
        variables["titleFilter"] = f"%{title_filter}%"
    while True:
        result = gql(q, variables)
        books = result.get("user_books", [])
        all_books.extend(books)
        if len(books) < LIBRARY_FETCH_LIMIT:
            break
        offset += LIBRARY_FETCH_LIMIT
        variables["offset"] = offset
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
            f"{Term.YELLOW}Warning: passing the token as a command-line argument may leave it in your "
            f"shell history. Consider running `hardcover.py setup` with no argument instead.{Term.RESET}",
            file=sys.stderr,
        )
    token = args.token or input("Paste your Hardcover API token: ").strip()
    if not token:
        print(f"{Term.RED}No token provided.{Term.RESET}", file=sys.stderr)
        sys.exit(1)
    save_token(token)


def cmd_whoami(args):
    me = get_me()

    def printer():
        print(f"\n  {Term.BOLD}{Term.B_CYAN}Hardcover Profile{Term.RESET}")
        print(f"  {Term.DIM}┌──────────────────────────────────────┐{Term.RESET}")
        print(f"    {Term.BOLD}Username{Term.RESET} : {me['username']}")
        print(f"    {Term.BOLD}ID{Term.RESET}       : {me['id']}")
        print(
            f"    {Term.BOLD}Books{Term.RESET}    : {Term.B_GREEN}{me['books_count']}{Term.RESET}"
        )
        print(f"  {Term.DIM}└──────────────────────────────────────┘{Term.RESET}\n")

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
                f"  {Term.YELLOW}No books found.{Term.RESET}"
                if not args.status
                else f"  {Term.YELLOW}No books with status '{args.status}'.{Term.RESET}"
            )
            return

        print(f"\n  {Term.BOLD}{Term.B_CYAN}Your Library{Term.RESET}")
        print(
            f"  {Term.DIM}┌────┬────────────────────┬────────┬──────────────────────────────────────────────┬────────────────────────┐{Term.RESET}"
        )
        print(
            f"  {Term.DIM}│{Term.RESET} {Term.BOLD}{'#':<2}{Term.RESET} {Term.DIM}│{Term.RESET} {Term.BOLD}{'Status':<18}{Term.RESET} {Term.DIM}│{Term.RESET} {Term.BOLD}{'Rating':<6}{Term.RESET} {Term.DIM}│{Term.RESET} {Term.BOLD}{'Title':<44}{Term.RESET} {Term.DIM}│{Term.RESET} {Term.BOLD}{'Authors':<22}{Term.RESET} {Term.DIM}│{Term.RESET}"
        )
        print(
            f"  {Term.DIM}├────┼────────────────────┼────────┼──────────────────────────────────────────────┼────────────────────────┤{Term.RESET}"
        )

        for i, ub in enumerate(books, 1):
            status_id = ub["status_id"]
            status_name = STATUS_MAP.get(status_id, "?")
            color = _status_color(status_id)
            styled_status = f"{color}{status_name:<18}{Term.RESET}"

            rating_val = ub.get("rating")
            if rating_val:
                rating_plain = f"★ {rating_val:.1f}"
                styled_rating = f"{Term.B_YELLOW}★ {rating_val:.1f}{Term.RESET}"
            else:
                rating_plain = "—"
                styled_rating = f"{Term.DIM}—{Term.RESET}"
            rating_cell = pad_styled(rating_plain, styled_rating, 6)

            authors_list = [
                c["author"]["name"] for c in ub["book"]["contributions"][:2]
            ]
            authors = ", ".join(authors_list)
            if len(ub["book"]["contributions"]) > 2:
                authors += "..."
            authors_trunc = authors[:22]
            authors_cell = pad_styled(authors_trunc, authors_trunc, 22)

            raw_title = ub["book"]["title"]
            owned = ub["owned"]
            max_title = 38 if owned else 44
            owned_suffix = f" {Term.GREEN}[own]{Term.RESET}" if owned else ""
            display_title = raw_title[: max_title - 3] + "..." if len(raw_title) > max_title else raw_title

            plain_title = display_title + (" [own]" if owned else "")
            styled_title = f"{display_title}{owned_suffix}"
            title_cell = pad_styled(plain_title, styled_title, 44)

            print(
                f"  {Term.DIM}│{Term.RESET} {Term.DIM}{i:<2}{Term.RESET} {Term.DIM}│{Term.RESET} {styled_status} {Term.DIM}│{Term.RESET} {rating_cell} {Term.DIM}│{Term.RESET} {title_cell} {Term.DIM}│{Term.RESET} {authors_cell} {Term.DIM}│{Term.RESET}"
            )

        print(
            f"  {Term.DIM}└────┴────────────────────┴────────┴──────────────────────────────────────────────┴────────────────────────┘{Term.RESET}"
        )
        print(
            f"  {Term.DIM}Showing {len(books)} books (offset={args.offset}){Term.RESET}\n"
        )

    print_or_json(args, books, printer)


def cmd_stats(args):
    me = get_me()
    user_id = me["id"]

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
        print(
            f"\n  {Term.BOLD}{Term.B_CYAN}Library Statistics: {me['username']}{Term.RESET}"
        )
        print(f"  {Term.DIM}┌────────────────────────────────────────┐{Term.RESET}")

        stats = [
            ("Total books", f"{total_count}", Term.RESET),
            ("Read", f"{read_count}", Term.B_GREEN),
            ("Currently reading", f"{reading_count}", Term.B_YELLOW),
            ("Want to read", f"{want_count}", Term.CYAN),
            ("DNF (Did Not Finish)", f"{dnf_count}", Term.RED),
            ("Total pages read", f"{total_pages:,}", Term.BOLD),
        ]

        for label, val, color in stats:
            dots_count = max(0, 38 - len(label) - len(val))
            print(
                f"  {Term.DIM}│{Term.RESET}  {label} {Term.DIM}{'.' * dots_count}{Term.RESET} {color}{val}{Term.RESET}  {Term.DIM}│{Term.RESET}"
            )

        if rated_count:
            rating_text = f"{avg_rating:.2f} ({rated_count} rated)"
            dots_count = max(0, 38 - len("Avg rating") - len(rating_text) - 2)
            print(
                f"  {Term.DIM}│{Term.RESET}  Avg rating {Term.DIM}{'.' * dots_count}{Term.RESET} {Term.B_YELLOW}★ {avg_rating:.2f}{Term.RESET} {Term.DIM}({rated_count} rated){Term.RESET}  {Term.DIM}│{Term.RESET}"
            )

        print(f"  {Term.DIM}└────────────────────────────────────────┘{Term.RESET}")
        print()

        if goals:
            print(f"  {Term.BOLD}{Term.B_CYAN}Active Goals{Term.RESET}")
            print(f"  {Term.DIM}──────────────────────────────────────────{Term.RESET}")
            for g in goals:
                pct = (float(g["progress"]) / g["goal"] * 100) if g["goal"] else 0
                bar = make_progress_bar(pct, width=24)
                metric_name = g["metric"].replace("_", " ").title()
                print(f"    {Term.BOLD}{metric_name}{Term.RESET}")
                print(
                    f"    {bar}  {Term.BOLD}{pct:.0f}%{Term.RESET}  {Term.DIM}({g['progress']}/{g['goal']}){Term.RESET}"
                )
                print(
                    f"    {Term.DIM}Timeframe: {g['start_date']} to {g['end_date']}{Term.RESET}\n"
                )

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
            print(f"  {Term.YELLOW}No books currently being read.{Term.RESET}")
            return
        print(
            f"\n  {Term.BOLD}{Term.B_CYAN}Currently Reading ({len(books)}){Term.RESET}"
        )
        print(
            f"  {Term.DIM}────────────────────────────────────────────────────────────{Term.RESET}"
        )
        for ub in books:
            book = ub["book"]
            total = effective_pages(ub)
            reads = ub.get("user_book_reads", [])
            current = reads[0]["progress_pages"] if reads else 0
            total_int = total if isinstance(total, int) else 0
            pct = (current / total_int * 100) if total_int else 0

            bar = make_progress_bar(pct, width=30)
            total_display = f"{total}" if total_int else "?"

            print(f"\n    {Term.BOLD}{book['title']}{Term.RESET}")
            print(
                f"    {bar}  {Term.BOLD}{pct:.0f}%{Term.RESET}  {Term.DIM}({current}/{total_display} pages){Term.RESET}"
            )
        print()

    print_or_json(args, books, printer)


def cmd_log(args):
    """Log reading progress, status change, or rating for a book."""
    if args.percent is not None and not (0 <= args.percent <= 100):
        print(
            f"{Term.RED}Error: --percent must be between 0 and 100.{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)
    if args.rating is not None and not (0 <= args.rating <= 5):
        print(
            f"{Term.RED}Error: --rating must be between 0 and 5.{Term.RESET}",
            file=sys.stderr,
        )
        sys.exit(1)

    me = get_me()

    user_book_id = args.user_book_id
    edition_id = None
    if user_book_id is None:
        data = fetch_all_user_books(
            me["id"],
            "id\nedition_id\nedition { pages }\nbook { id title pages }",
            title_filter=args.book,
        )
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
            print(
                f"{Term.RED}No book matching '{args.book}' in your library.{Term.RESET}"
            )
            sys.exit(1)
        if len(matches) > 1:
            print(f"{Term.YELLOW}Multiple matches for '{args.book}':{Term.RESET}")
            for i, ub in enumerate(matches, 1):
                print(f"  [{i}] {ub['book']['title']}")
            if sys.stdin.isatty():
                try:
                    choice = input("Select a book (number): ").strip()
                    idx = int(choice) - 1
                    if idx < 0 or idx >= len(matches):
                        raise ValueError
                except (ValueError, EOFError):
                    print(
                        f"{Term.RED}Invalid choice.{Term.RESET}",
                        file=sys.stderr,
                    )
                    sys.exit(1)
                selected_ub = matches[idx]
            else:
                print(
                    f"  Use --id <id> to specify which book.",
                    file=sys.stderr,
                )
                sys.exit(1)
        else:
            selected_ub = matches[0]
        user_book_id = selected_ub["id"]
        edition_id = selected_ub.get("edition_id")
        book_title = selected_ub["book"]["title"]
        book_pages = effective_pages(selected_ub)
    else:
        book_title = f"user_book #{user_book_id}"
        book_pages = None
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
                print(
                    f"{Term.RED}Error: user_book #{user_book_id} not found.{Term.RESET}",
                    file=sys.stderr,
                )
                sys.exit(1)
            edition_id = selected_ub.get("edition_id")
            book_pages = effective_pages(selected_ub)

    result = {"book": book_title, "user_book_id": user_book_id}

    if args.percent is not None:
        if not book_pages:
            print(
                f"{Term.RED}Could not determine page count for '{book_title}'. Use --pages instead.{Term.RESET}"
            )
            sys.exit(1)
        args.pages = round(book_pages * args.percent / 100)

    if args.pages is not None and args.pages < 0:
        print(
            f"{Term.RED}Error: --pages cannot be negative.{Term.RESET}", file=sys.stderr
        )
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

        result["pages_logged"] = args.pages

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
        result["status"] = STATUS_MAP[status_id]

    if args.rating is not None:
        m = """
        mutation ($id: Int!, $object: UserBookUpdateInput!) {
          update_user_book(id: $id, object: $object) {
            id
          }
        }
        """
        gql(m, {"id": user_book_id, "object": {"rating": args.rating}})
        result["rating"] = args.rating

    if args.pages is None and args.status is None and args.rating is None:
        if getattr(args, "json", False):
            print(json.dumps({"error": "Nothing to log. Use --pages, --status, or --rating."}))
        else:
            print(
                f"{Term.YELLOW}Nothing to log. Use --pages, --status, or --rating.{Term.RESET}"
            )
        sys.exit(1)

    if getattr(args, "json", False):
        print(json.dumps(result, indent=2))
        return

    if result.get("pages_logged") is not None:
        print(
            f"  {Term.GREEN}✓{Term.RESET} Logged {Term.BOLD}{result['pages_logged']}{Term.RESET} pages for '{Term.BOLD}{book_title}{Term.RESET}'"
        )
    if result.get("status") is not None:
        print(
            f"  {Term.GREEN}✓{Term.RESET} Updated '{Term.BOLD}{book_title}{Term.RESET}' status to '{Term.CYAN}{result['status']}{Term.RESET}'"
        )
    if result.get("rating") is not None:
        print(
            f"  {Term.GREEN}✓{Term.RESET} Rated '{Term.BOLD}{book_title}{Term.RESET}' {Term.B_YELLOW}★ {result['rating']}{Term.RESET}/5"
        )


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
            print(f"  {Term.YELLOW}No results for '{args.query}'.{Term.RESET}")
            return
        print(
            f'\n  {Term.BOLD}{Term.B_CYAN}Search Results for "{args.query}"{Term.RESET}'
        )
        print(
            f"  {Term.DIM}────────────────────────────────────────────────────────────────────────{Term.RESET}"
        )
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

            print(f"  {Term.B_CYAN}{i:>2}.{Term.RESET} {Term.BOLD}{title}{Term.RESET}")
            meta = []
            if author:
                meta.append(f"by {Term.BOLD}{author}{Term.RESET}")
            if pages and pages != "?":
                meta.append(f"{pages}p")
            if year and year != "?":
                meta.append(f"{year}")
            if rating and rating != "?":
                meta.append(f"{Term.B_YELLOW}★ {rating}{Term.RESET}")

            print(f"     {Term.DIM}│{Term.RESET} {'  •  '.join(meta)}")
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
            print(f"  {Term.YELLOW}No reading goals found.{Term.RESET}")
            return
        print(f"\n  {Term.BOLD}{Term.B_CYAN}Reading Goals{Term.RESET}")
        print(
            f"  {Term.DIM}────────────────────────────────────────────────────────────{Term.RESET}"
        )
        for g in goals:
            pct = (float(g["progress"]) / g["goal"] * 100) if g["goal"] else 0
            bar = make_progress_bar(pct, width=25)

            state_colors = {
                "active": Term.B_CYAN,
                "completed": Term.B_GREEN,
                "failed": Term.RED,
            }
            color = state_colors.get(g["state"], Term.DIM)
            state_tag = f"{color}[{g['state'].upper()}]{Term.RESET}"

            metric_display = g["metric"].replace("_", " ").title()
            print(f"\n  {state_tag}  {Term.BOLD}{metric_display} Goal{Term.RESET}")
            print(
                f"     {bar}  {Term.BOLD}{pct:.0f}%{Term.RESET}  {Term.DIM}({g['progress']}/{g['goal']}){Term.RESET}"
            )
            print(
                f"     {Term.DIM}Period: {g['start_date']} ➔ {g['end_date']}{Term.RESET}"
            )
            if g.get("description"):
                print(f"     {Term.DIM}Note: {g['description']}{Term.RESET}")
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

    if getattr(args, "json", False):
        print(json.dumps(events, indent=2))
        return

    out = args.output
    with open(out, "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(
            f, ["book", "date", "timestamp", "cumulative_pages", "pages_read"]
        )
        w.writeheader()
        w.writerows(events)

    print(
        f"  {Term.GREEN}✓{Term.RESET} Wrote {Term.BOLD}{len(events)}{Term.RESET} events to {Term.BOLD}{out}{Term.RESET}"
    )
    print(f"\n  {Term.BOLD}Pages read per day:{Term.RESET}")
    for dt, books in sorted(daily.items()):
        print(
            f"    {Term.B_CYAN}●{Term.RESET} {Term.BOLD}{dt}{Term.RESET}: {Term.GREEN}{sum(books.values())} pages{Term.RESET}"
        )
        for book, pages in books.items():
            print(f"      {Term.DIM}├─{Term.RESET} {book}: {pages}p")


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
            print(
                f"  {Term.YELLOW}No reading activity in the last {args.days} days.{Term.RESET}"
            )

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
        print(
            f"\n  {Term.BOLD}{Term.B_CYAN}Reading Log (last {args.days} days){Term.RESET}"
        )
        print(
            f"  {Term.DIM}────────────────────────────────────────────────────────────────────────────────{Term.RESET}"
        )

        for idx, entry in enumerate(json_result):
            day_total = entry["total_pages"]
            total_all += day_total
            wd = weekday_names[datetime.fromisoformat(entry["date"]).weekday()]

            print(
                f"\n  {Term.B_CYAN}●{Term.RESET} {Term.BOLD}{entry['date']} {wd}{Term.RESET}  {Term.GREEN}+{day_total} pages{Term.RESET}"
            )

            for b_idx, b in enumerate(entry["books"]):
                cumul = b["cumulative"]
                total = b["total_book_pages"]
                cumul_str = f"{cumul}/{total}" if total else str(cumul)

                is_last_book = b_idx == len(entry["books"]) - 1
                tree_char = "└──" if is_last_book else "├──"

                dots = max(2, 45 - len(b["title"]))
                title_part = f"{b['title']} {Term.DIM}{'.' * dots}{Term.RESET}"
                pages_part = f"{Term.BOLD}{b['pages']}p{Term.RESET}"
                cumul_part = f"{Term.DIM}(cumulative {cumul_str}){Term.RESET}"

                print(
                    f"    {Term.DIM}{tree_char}{Term.RESET} {title_part} {pages_part}  {cumul_part}"
                )

        avg = total_all // len(json_result) if json_result else 0
        print(
            f"\n  {Term.DIM}────────────────────────────────────────────────────────────────────────────────{Term.RESET}"
        )
        print(
            f"  {Term.BOLD}Summary:{Term.RESET} {Term.GREEN}{total_all} pages{Term.RESET} over {Term.BOLD}{len(json_result)}{Term.RESET} active days {Term.DIM}│{Term.RESET} Avg: {Term.B_GREEN}{avg} pages/day{Term.RESET}"
        )
        print()

    print_or_json(args, json_result, printer)


# ── CLI ─────────────────────────────────────────────────────────────────────


def add_json_flag(p):
    p.add_argument(
        "--json", action="store_true", help="Output raw JSON instead of formatted text"
    )


def main():
    global DISABLE_COLOR
    if "--no-color" in sys.argv:
        DISABLE_COLOR = True
        _disable_color()

    parser = argparse.ArgumentParser(
        prog="hardcover",
        description="Manage your hardcover.app library from the terminal.",
    )
    parser.add_argument(
        "--version", action="version", version=f"%(prog)s {VERSION}"
    )
    parser.add_argument(
        "--no-color",
        action="store_true",
        help="Disable colored output (also respects NO_COLOR env var)",
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
    add_json_flag(p)
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
    add_json_flag(p)
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
