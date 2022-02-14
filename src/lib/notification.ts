import httpClient from "../utils/http";
import { GET_NOTI_RECEVIER, GET_SUBMISSION_USER_ID } from "../utils/queries";

export async function getNotiRecevier(assignmentConfigId: number){
    try {
        const { data: {data} } = await httpClient.request({
          url: '/graphql',
          data: {
            query: GET_NOTI_RECEVIER,
            variables: {
              assignmentConfigId
            }
          }
        });
        console.log(data);
        const ids = data.section_user.map((user:any)=>{return(user.user_id)})
        return ids
      } catch (error) {
        console.error(error);
        throw error;
      }
}

export async function getSubmissionUserId(submissionId: number){
  try{
    const {data:{data}} = await httpClient.request({
      url: '/graphql',
      data:{
        query: GET_SUBMISSION_USER_ID,
        variables:{
          submissionId
        }
      }
    })
    console.log(data);
    return data.submissions[0].user_id
  }catch (error) {
    console.error(error);
    throw error;
  }
}