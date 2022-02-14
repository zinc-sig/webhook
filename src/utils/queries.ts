export const GET_SUBMISSION_USER_ID = `
  query getUserId($submissionId:bigint!){
    submissions(where:{
      id:{_eq:$submissionId}
    }){
      user_id
    }
  }
`

export const GET_REQUIRED_CACHE_DATA_BY_SUBMISSION_ID = `
query ($id :bigint!) {
  submission(id :$id) {
    user{
      name
    }
    isLate
    created_at
    assignment_config{
      dueAt
    }
  }
}
`

export const GET_GRADING_SUBMISSIONS = `
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
  }
`

export const GET_SELECTED_SUBMISSIONS = `
  query getSelectedSubmissions($submissions: [bigint!]!) {
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
  }
`

export const GET_LATEST_SUBMISSIONS_FOR_ASSIGNMENT_CONFIG = `
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
  }
`

export const UPDATE_DECOMPRESSION_RESULT_FOR_SUBMISSION = `
  mutation updateDecompressionResult($id: bigint!, $extractedPath: String, $failReason: String) {
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
  }
`

export const GET_GRADING_POLICY = `
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
  }
`

export const UPDATE_USERNAME = `
  mutation updateUser($id: bigint!, $name: String!) {
    updateUser(pk_columns: {
      id: $id
    } _set: {
      name: $name
    }) {
      updatedAt
    }
  }
`

export const GET_USER = `
  query getUser($itsc: String!) {
    users(where: {
      itsc: {
        _eq: $itsc
      }
    }) {
      id
      name
      isAdmin
      hasTeachingRole
      courses {
        course_id
      }
    }
  }
`

export const CREATE_USER = `
  mutation createUserIfNotExist($itsc:String!, $name:String!) {
    createUser(
      object:{
        itsc: $itsc
        name: $name
      }
    ){ id }
  }
`

export const ADD_REPORT_ARTIFACTS = `
  mutation addProccessedReportArtifact($id: bigint!, $grade: jsonb, $sanitizedReports: jsonb) {
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
  }
`

export const GET_NOTI_RECEVIER = `
  query findNotiRecevier($assignmentConfigId: jsonb){
    section_user(where:{
      assignment_config_ids:{
        _contains: $assignmentConfigId
      }
    }){
      user_id
    }
  }
`

export const GET_USER_BY_REPORT_ID = `
  query getUserByReportId ($report_id : bigint!) { 
    report(id : $report_id){ 
      submission { 
        user {
          itsc 
          name
        }
      }
    }
  }
`

export const GET_ASSIGNMENT_CONFIG_ID_BY_SUBMISSON_ID = `
  query getAssignmentConfigIdBySubmissionId ($id : bigint!) { 
    submissions (id : $id ) {
      assignment_config_id
    }
  }
`