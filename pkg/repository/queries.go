package repository

const GetUserByITSC = `
query GetUserByITSC($itsc: String!) {
  users(where: {itsc: {_eq: $itsc}}) {
    id
    name
		itsc
    isAdmin
    hasTeachingRole
    courses {
      course_id
    }
  }
}
`

const CreateUser = `
mutation CreateUser($itsc: String!, $name: String!) {
  createUser(
    object:{
      itsc: $itsc
      name: $name
    }
    on_conflict: {
      constraint: users_itsc_key
      update_columns: [createdAt]
    }
  ){ id }
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
  updateSubmission(
    pk_columns: {
      id: $id
    },
    _set: {
      extracted_path: $extractedPath
      fail_reason: $failReason
    }
  ) {
    id
  }
}`

const getGradingPolicy = `
query getGradingPolicy($id: bigint!, $userId: bigint!) {
  assignmentConfig(id: $id) {
    gradeImmediately
    assignment {
      course {
        users(where: {
          user_id: {
            _eq: $userId
          }
        }) {
          permission
        }
      }
    }
  }
}`

const createSubmission = `
mutation addSubmissionEntry($submission: submissions_insert_input!) {
  createSubmission(
    object: $submission
  ){ id }
}
`

const getGradingSubmissions = `
query getGradingSubmissions($assignmentConfigId: bigint!) {
  assignmentConfig(id: $assignmentConfigId) {
    stopCollectionAt
    submissions(
      distinct_on: [user_id]
      order_by: [
        { user_id: desc }
        { created_at: desc }
      ]
      where: {
        extracted_path: {
          _is_null: false
        }
      }
    ) {
      id
      extracted_path
      created_at
    }
  }
}`

const getLatestSubmissionsForAssignmentConfig = `
query getLatestSubmissionsForAssignmentConfig($assignmentConfigId: bigint!) {
  assignmentConfig(id: $assignmentConfigId) {
    submissions(
      distinct_on: [user_id]
      order_by: [
        { user_id: desc }
        { created_at: desc }
      ]
      where: {
        extracted_path: {
          _is_null: false
        }
      }
    ) {
      id
      extracted_path
      created_at
    }
  }
}`

const getSubmissionGrades = `
query getSubmissionsForAssignmentConfig($id: bigint!) {
  assignmentConfig(id: $id) {
    dueAt
    assignment {
      name
    }
    submissions(
      distinct_on: [user_id]
      order_by: [
        { user_id: asc }
        { created_at: desc }
      ]
    ) {
      id
      isLate
      created_at
      reports(
        limit: 1
        order_by: {
          createdAt: desc
        }
      ) {
        grade
      }
      user {
        itsc
        name
      }
    }
  }
}`

const getSubmissionByID = `
query getSubmission($id: bigint!) {
  submission(
    id: $id
  ){
    stored_name
    upload_name
    created_at
  }
}`

const getAllSubmissionsForAssignmentConfig = `
query getSubmissionsForAssignmentConfig($assignmentConfigId: bigint!) {
  assignmentConfig(id: $assignmentConfigId) {
    assignment {
      course {
        code
        semester {
          year
          term
        }
      }
    }
    submissions(
      distinct_on: [user_id]
      order_by: [
        { user_id: asc }
        { created_at: desc }
      ]
    ) {
      stored_name
      upload_name
      user {
        itsc
      }
    }
  }
}`

const getSelectedSubmissions = `
query getSelectedSubmissions($submissions: [bigint!]) {
   submissions(
    where: {
      id: {
        _in: $submissions
      }
    }
  ) {
    id
    extracted_path
    created_at
  }
}`

const addReportArtifacts = `
mutation addReportArtifacts($id: bigint!, $sanitizedReports: jsonb!, $grade: jsonb!) {
  updateReport(
    pk_columns: {
      id: $id
    }
    _set: {
      grade: $grade
      sanitizedReports: $sanitizedReports
    }
  ) {
    id
  }
}`
