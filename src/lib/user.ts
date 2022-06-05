import jwksClient from "jwks-rsa";
import jwt from "jsonwebtoken";
import httpClient from "../utils/http";
import { CREATE_USER, GET_USER, UPDATE_USERNAME , GET_USER_BY_REPORT_ID} from "../utils/queries";

const client = jwksClient({
  jwksUri: `https://login.microsoftonline.com/common/discovery/v2.0/keys`
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
    const { email, name } = await new Promise((resolve, reject) => {
      jwt.verify(
        idToken,
        getKey,
        {
          issuer: `https://login.microsoftonline.com/${process.env.AAD_TENANT_ID}/v2.0`,
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
    const firstName = name.substring(0, name.lastIndexOf(' '));
    const lastName = name.substring(name.lastIndexOf(' ') + 1);
    return {
      itsc: email.split('@')[0],
      name: `${lastName}, ${firstName}`
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
