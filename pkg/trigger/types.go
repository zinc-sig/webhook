package trigger

type EnrollmentMap struct {
	Term     string `json:"term"`
	CrseCode string `json:"crseCode"`
	Classes  []struct {
		CrseTitle string `json:"crseTitle"`
		Section   string `json:"section"`
		ClassType string `json:"classType"`
		Students  []struct {
			EmailAddr    string `json:"emailAddr"`
			EnrollStatus string `json:"enrollStatus"`
		} `json:"students"`
	} `json:"classes"`
}

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
