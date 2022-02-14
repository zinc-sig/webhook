import jwksClient from "jwks-rsa";
import jwt from "jsonwebtoken";
import httpClient from "../utils/http";
import { CREATE_USER, GET_USER, UPDATE_USERNAME , GET_USER_BY_REPORT_ID} from "../utils/queries";

const client = jwksClient({
  jwksUri: `https://cas${process.env.NODE_ENV==='production'?'':'test'}.ust.hk/cas/oidc/jwks`
});

function getKey(header: any, callback: (...args: any) => void) {
  client.getSigningKey(header.kid, function(_, key) {
    const signingKey =
      (key as jwksClient.CertSigningKey).publicKey ||
      (key as jwksClient.RsaSigningKey).rsaPublicKey;
    callback(null, signingKey);
  });
}

export async function verifySignature(idToken: string, audience: string): Promise<any> {
  try {
    console.log(client)
    const { sub, name } = await new Promise((resolve, reject) => {
      jwt.verify(
        idToken,
        getKey,
        {
          issuer: process.env.CAS_AUTH_BASE_URL,
          audience, 
          algorithms: ["RS256"]
        },
        (error, decoded: any) => {
          if (error) {
            reject(error);
          }
          if (decoded) {
            resolve(decoded);
          }
        }
      );
    });
    return {
      itsc: sub,
      name
    }
  } catch (error) {
    throw error;
  }
}

export async function updateUserName(id: number, name: string): Promise<any> {
  try {
    await httpClient.request({
      url: '/graphql',
      data: {
        query: UPDATE_USERNAME,
        variables: {
          id, name
        }
      }
    });
  } catch (error) {
    throw error;
  }
}

export async function getUser(itsc: string, name: string): Promise<any> {
  try{
    const { data:{data} } = await httpClient.request({
      url: '/graphql',
      data: {
        query: GET_USER,
        variables: { itsc }
      }
    });
    const exist = data.users.length>0;
    if (exist) {
      const [user] = data.users;
      if(!user.name) {
        updateUserName(user.id, name);
      }
      return user;
    }
    else {
      console.log(`[!] User ${name} does not exist in Zinc, creating account for itsc ${itsc}`)
      await createUser(itsc, name);
      return {
        isAdmin: false
      };
    }
  }catch(error){
    throw error;
  }
}

async function createUser(itsc: string, name: string): Promise<any> {
  try {
    const { data } = await httpClient.request({
      url: '/graphql',
      data: {
        query: CREATE_USER,
        variables: { itsc, name }
      },
    });
    console.log(`[!] Created new user for itsc id: ${itsc}`)
    const { data: { createUser }} = data;
    return createUser;
  } catch (error) {
    throw error;
  }
}

export async function getUserByReportId(report_id : string ) : Promise <any> { 
  const {data : {data}} = await httpClient.request (
    {
      url : '/graphql' ,
      data : {
        query : GET_USER_BY_REPORT_ID , 
        variable : {report_id } 
      }
    }
  ) ; 
  const exist = data.users.length > 0 ; 
  if (exist ) { 
    const [user ] = data.users ; 
    return user ; 
  }
  else {
    console.log(`[!] User cannot be found by report id in Zinc`)
    return null  ; 
  }
}