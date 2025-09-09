package repository

const GetUserByITSC = `
query GetUserByITSC($itsc: String!) {
  users(where: {itsc: {_eq: $itsc}}) {
    id
    name
		itsc
    isAdmin
    courses {
      course_id
    }
  }
}
`

const CreateUser = `
mutation CreateUser($itsc: String!, $name: String!) {
  insert_users_one(object: {itsc: $itsc, name: $name}) {
    id
  }
}
`

const createSemester = `
mutation createSemester($id: bigint!, $name: String!, $year: Int!) {
  createSemester(object: {id: $id, name: $name, year: $year}, on_conflict: {constraint: semesters_pkey, update_columns: [updatedAt]}) {
    createdAt
    updatedAt
  }
}`

const addCourse = `
mutation addCourse($code: String!, $semesterId: bigint!, $name: String!) {
  createCourse(object: {code: $code, semester_id: $semesterId, name: $name}, on_conflict: {constraint: courses_code_semester_id_key, update_columns: updated_at}) {
    id
    semester {
      name
    }
  }
}`

const addSections = `
mutation batchCreateSection($sections: [sections_insert_input!]!) {
  batchCreateSection(objects: $sections, on_conflict: {constraint: sections_course_id_name_key, update_columns: updated_at}) {
    returning {
      id
      name
      course {
        code
      }
    }
  }
}`

const addStudentsToCourseSection = `
mutation addUsersToSection($users: [section_user_insert_input!]!) {
  addUsersToSection(objects: $users) {
    affected_rows
  }
}`

const addStudentsToCourse = `
mutation enrollUsersToCourse($users: [course_user_insert_input!]!) {
  enrollUsersInCourse(objects: $users) {
    affected_rows
  }
}`

const getStudentUserIds = `
query getUserIds($itscIds: [String!]!) {
  users(where: {itsc: {_in: $itscIds}}) {
    id
    itsc
  }
}`

const addUsers = `
mutation registerUsers($users: [users_insert_input!]!) {
  batchCreateUser(objects: $users) {
    returning {
      id
      itsc
    }
    affected_rows
  }
}`

const removeStudentsFromCourse = `
mutation removeStudentsFromCourse($courseId: bigint!) {
  removeUsersFromCourse(where: {course_id: {_eq: $courseId}, permission: {_eq: 1}}) {
    affected_rows
  }
}`

const removeStudentsFromSection = `
mutation removeStudentsFromSection($courseId: bigint!) {
  removeStudentsFromSection(where: {section: {course_id: {_eq: $courseId}}}) {
    affected_rows
  }
}`

const updateDecompressionResultForSubmission = `
mutation updateDecompressionResultForSubmission($id: bigint!, $extractedPath: String, $failReason: String) {
  update_submissions_by_pk(pk_columns: {id: $id}, _set: {extracted_path: $extractedPath, fail_reason: $failReason}) {
    id
  }
}`

const getGradingSubmissions = `
query getGradingSubmissions($assignmentConfigId: bigint!) {
  assignmentConfig: assignment_configs_by_pk(id: $assignmentConfigId) {
    stopCollectionAt: stop_collection_at
    submissions(distinct_on: user_id, order_by: {user_id: asc, created_at: desc}) {
      id
      extracted_path
      created_at
    }
  }
}`

const getLatestSubmissionsForAssignmentConfig = `
query getLatestSubmissionsForAssignmentConfig($assignmentConfigId: bigint!) {
  assignmentConfig: assignment_configs_by_pk(id: $assignmentConfigId) {
    submissions(distinct_on: user_id, order_by: {user_id: asc, created_at: desc}) {
      id
      extracted_path
      created_at
    }
  }
}`

const getSelectedSubmissions = `
query getSelectedSubmissions($submissions: [bigint!]) {
  submissions(where: {id: {_in: $submissions}}) {
    id
    extracted_path
    created_at
  }
}`

const addReportArtifacts = `
mutation addReportArtifacts($id: bigint!, $sanitizedReports: jsonb!, $grade: jsonb!) {
  update_reports_by_pk(pk_columns: {id: $id}, _set: {sanitized_pipeline_results: $sanitizedReports, grade: $grade}) {
    id
  }
}`
