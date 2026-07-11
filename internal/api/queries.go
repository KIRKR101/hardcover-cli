package api

// All GraphQL queries/mutations used by the CLI live here as constants.
// Keeping them in one place makes them easy to find and audit.

const QueryMe = `
query {
  me {
    id
    username
    books_count
  }
}
`

// QueryLibrary is the user_books query used by the library command.
const QueryLibrary = `
query ($userId: Int!, $limit: Int!, $offset: Int!, $statusId: Int) {
  user_books(
    where: { user_id: { _eq: $userId }, status_id: { _eq: $statusId } }
    order_by: { date_added: desc }
    limit: $limit
    offset: $offset
  ) {
    id
    status_id
    rating
    date_added
    owned
    book {
      id
      title
      pages
      contributions {
        author { name }
      }
    }
  }
}
`

// QueryLibraryNoStatus is the variant when no --status filter is applied.
const QueryLibraryNoStatus = `
query ($userId: Int!, $limit: Int!, $offset: Int!) {
  user_books(
    where: { user_id: { _eq: $userId } }
    order_by: { date_added: desc }
    limit: $limit
    offset: $offset
  ) {
    id
    status_id
    rating
    date_added
    owned
    book {
      id
      title
      pages
      contributions {
        author { name }
      }
    }
  }
}
`

// QueryUserBookByID is used by log --id to fetch a single user_book.
const QueryUserBookByID = `
query ($id: Int!) {
  user_books_by_pk(id: $id) {
    id
    edition_id
    status_id
    rating
    edition { id pages }
    book { id title pages }
  }
}
`

// QueryUserBooksByTitle is used by log to fuzzy-match a book.
// Hardcover restricts _ilike on book.title, so we fetch the user's
// library and filter on the client side. The book fields are
// minimally populated to keep payload size reasonable.
const QueryUserBooksByTitle = `
query ($userId: Int!, $limit: Int!, $offset: Int!) {
  user_books(
    where: { user_id: { _eq: $userId } }
    order_by: { updated_at: desc }
    limit: $limit
    offset: $offset
  ) {
    id
    edition_id
    status_id
    rating
    owned
    edition { id pages }
    book { id title pages }
  }
}
`

// QueryStats returns aggregates and active goals.
const QueryStats = `
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
    aggregate { count avg { rating } }
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
`

// QueryReadBooksPages fetches just edition/book pages for read books,
// paginated. Used by the stats command to compute total pages.
const QueryReadBooksPages = `
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
`

// QueryProgress is used by the progress command.
const QueryProgress = `
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
`

// QuerySearch hits Hardcover's search endpoint.
const QuerySearch = `
query ($query: String!, $perPage: Int!) {
  search(query: $query, query_type: "Book", per_page: $perPage, page: 1) {
    results
  }
}
`

// QueryGoalsAll lists every goal (including archived). Caller decides
// whether to filter archived in code or use QueryGoalsActive.
const QueryGoalsAll = `
query ($userId: Int!) {
  goals(
    where: { user_id: { _eq: $userId } }
    order_by: { start_date: desc }
  ) {
    id
    metric
    goal
    progress
    state
    start_date
    end_date
    description
  }
}
`

// QueryGoalsActive lists only non-archived goals.
const QueryGoalsActive = `
query ($userId: Int!) {
  goals(
    where: { user_id: { _eq: $userId }, archived: { _eq: false } }
    order_by: { start_date: desc }
  ) {
    id
    metric
    goal
    progress
    state
    start_date
    end_date
    description
  }
}
`

// QueryJournals fetches reading journal events for export/daily.
const QueryJournals = `
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
`

// QueryBookTitles fetches id and title for every user book, paginated.
// Used by export to build the book_id -> title mapping.
const QueryBookTitles = `
query ($userId: Int!, $limit: Int!, $offset: Int!) {
  user_books(
    where: { user_id: { _eq: $userId } }
    limit: $limit
    offset: $offset
  ) {
    book { id title }
  }
}
`

// QueryBookTitlesAndPages fetches id, title, and pages for every user
// book, paginated. Used by daily to build book metadata.
const QueryBookTitlesAndPages = `
query ($userId: Int!, $limit: Int!, $offset: Int!) {
  user_books(
    where: { user_id: { _eq: $userId } }
    limit: $limit
    offset: $offset
  ) {
    book { id title pages }
  }
}
`

// QueryActiveRead fetches the active user_book_read for a user_book.
const QueryActiveRead = `
query ($userBookId: Int!) {
  user_book_reads(
    where: {
      user_book_id: { _eq: $userBookId }
      finished_at: { _is_null: true }
    }
    order_by: [{ started_at: desc_nulls_last }, { id: desc }]
    limit: 1
  ) {
    id
  }
}
`

// MutationUpdateUserBookRead updates the active user_book_read.
const MutationUpdateUserBookRead = `
mutation ($id: Int!, $object: DatesReadInput!) {
  update_user_book_read(id: $id, object: $object) {
    id
  }
}
`

// MutationInsertUserBookRead creates a new user_book_read.
const MutationInsertUserBookRead = `
mutation ($userBookId: Int!, $userBookRead: DatesReadInput!) {
  insert_user_book_read(user_book_id: $userBookId, user_book_read: $userBookRead) {
    id
  }
}
`

// MutationUpdateUserBook updates status_id and/or rating.
const MutationUpdateUserBook = `
mutation ($id: Int!, $object: UserBookUpdateInput!) {
  update_user_book(id: $id, object: $object) {
    id
  }
}
`

// MutationInsertUserBook adds a book to the user's library.
const MutationInsertUserBook = `
mutation ($object: UserBookCreateInput!) {
  insert_user_book(object: $object) {
    id
    error
  }
}
`

// QueryBookEditions fetches editions for a given book, ordered by
// popularity (users_count). Used by the add command so the user can
// pick a specific edition.
const QueryBookEditions = `
query ($bookId: Int!, $limit: Int!) {
  books_by_pk(id: $bookId) {
    editions(
      limit: $limit
      order_by: { users_count: desc }
    ) {
      id
      pages
      edition_format
      physical_format
      release_year
      title
      isbn_13
      isbn_10
      users_count
      publisher { name }
      language { language }
      reading_format { format }
    }
  }
}
`
